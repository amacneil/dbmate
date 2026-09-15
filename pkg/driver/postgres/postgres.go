package postgres

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/lib/pq"
)

// pgDumpVersion represents a parsed pg_dump version
type pgDumpVersion struct {
	major int
	minor int
}

// pgDumpVersionRegexp matches pg_dump version output like "pg_dump (PostgreSQL) 17.6 (Debian 17.6-1.pgdg120+1)"
var pgDumpVersionRegexp = regexp.MustCompile(`\(PostgreSQL\) (\d+)\.(\d+)`)

// getPgDumpVersion returns the version of pg_dump, or nil if it cannot be determined
func getPgDumpVersion() *pgDumpVersion {
	cmd := exec.Command("pg_dump", "--version")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	matches := pgDumpVersionRegexp.FindStringSubmatch(string(output))
	if len(matches) < 3 {
		return nil
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil
	}

	return &pgDumpVersion{major: major, minor: minor}
}

// supportsRestrictKey returns true if pg_dump supports --restrict-key.
func (v *pgDumpVersion) supportsRestrictKey() bool {
	if v == nil {
		return false
	}
	// --restrict-key was added in PostgreSQL 15.14, 16.10, and 17.6
	return v.major > 17 ||
		(v.major == 17 && v.minor >= 6) ||
		(v.major == 16 && v.minor >= 10) ||
		(v.major == 15 && v.minor >= 14)
}

func init() {
	dbmate.RegisterDriver(NewDriver, "postgres")
	dbmate.RegisterDriver(NewDriver, "postgresql")
	dbmate.RegisterDriver(NewDriver, "redshift")
	dbmate.RegisterDriver(NewDriver, "spanner-postgres")
}

// Driver provides top level database functions
type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
}

// NewDriver initializes the driver
func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
	}
}

func connectionString(u *url.URL) string {
	hostname := u.Hostname()
	port := u.Port()
	query := u.Query()

	// support socket parameter for consistency with mysql
	if query.Get("socket") != "" {
		query.Set("host", query.Get("socket"))
		query.Del("socket")
	}

	// default hostname
	if hostname == "" && query.Get("host") == "" {
		switch runtime.GOOS {
		case "linux":
			query.Set("host", "/var/run/postgresql")
		case "darwin", "freebsd", "dragonfly", "openbsd", "netbsd":
			query.Set("host", "/tmp")
		default:
			hostname = "localhost"
		}
	}

	// host param overrides url hostname
	if query.Get("host") != "" {
		hostname = ""
	}

	// always specify a port
	if query.Get("port") != "" {
		port = query.Get("port")
		query.Del("port")
	}
	if port == "" {
		switch u.Scheme {
		case "redshift":
			port = "5439"
		default:
			port = "5432"
		}
	}

	// generate output URL
	out, _ := url.Parse(u.String())
	// force scheme back to postgres if there was another postgres-compatible scheme
	out.Scheme = "postgres"
	out.Host = fmt.Sprintf("%s:%s", hostname, port)
	out.RawQuery = query.Encode()

	return out.String()
}

func connectionArgsForDump(conn *url.URL, userArgs ...string) []string {
	u, err := url.Parse(connectionString(conn))
	if err != nil {
		panic(err)
	}

	// find schemas from search_path
	query := u.Query()
	schemas := strings.Split(query.Get("search_path"), ",")
	query.Del("search_path")
	query.Del("binary_parameters")
	u.RawQuery = query.Encode()

	out := []string{}
	for _, schema := range schemas {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			out = append(out, "--schema", schema)
		}
	}

	out = append(out, userArgs...)
	out = append(out, u.String())
	return out
}

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("postgres", connectionString(drv.databaseURL))
}

func (drv *Driver) openPostgresDB() (*sql.DB, error) {
	// clone databaseURL
	postgresURL, err := url.Parse(connectionString(drv.databaseURL))
	if err != nil {
		return nil, err
	}

	// connect to postgres database, unless this is a Redshift connection
	if drv.databaseURL.Scheme != "redshift" {
		postgresURL.Path = "postgres"
	}

	return sql.Open("postgres", postgresURL.String())
}

// CreateDatabase creates the specified database
func (drv *Driver) CreateDatabase() error {
	name := dbutil.DatabaseName(drv.databaseURL)
	fmt.Fprintf(drv.log, "Creating: %s\n", name)

	db, err := drv.openPostgresDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *Driver) DropDatabase() error {
	name := dbutil.DatabaseName(drv.databaseURL)
	fmt.Fprintf(drv.log, "Dropping: %s\n", name)

	db, err := drv.openPostgresDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		pq.QuoteIdentifier(name)))

	return err
}

func (drv *Driver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return nil, err
	}

	// load applied migrations
	migrations, err := dbutil.QueryColumn(db,
		"select quote_literal(version) from "+migrationsTable+" order by version asc")
	if err != nil {
		return nil, err
	}

	// build migrations table data
	var buf bytes.Buffer
	buf.WriteString("\n--\n-- Dbmate schema migrations\n--\n\n")

	if len(migrations) > 0 {
		buf.WriteString("INSERT INTO " + migrationsTable + " (version) VALUES\n    (" +
			strings.Join(migrations, "),\n    (") +
			");\n")
	}

	return buf.Bytes(), nil
}

// DumpSchema returns the current database schema
func (drv *Driver) DumpSchema(db *sql.DB, extraArgs ...string) ([]byte, error) {
	// load schema
	args := []string{"--format=plain", "--encoding=UTF8", "--schema-only",
		"--no-privileges", "--no-owner"}

	// PostgreSQL 15.14+/16.10+/17.6+ adds \restrict/\unrestrict commands to pg_dump output with a random key
	// by default, making the output non-deterministic. Use a fixed key for reproducible output.
	// See: https://github.com/amacneil/dbmate/issues/678
	if version := getPgDumpVersion(); version.supportsRestrictKey() {
		args = append(args, "--restrict-key=dbmate")
	}

	args = append(args, connectionArgsForDump(drv.databaseURL, extraArgs...)...)
	schema, err := dbutil.RunCommand("pg_dump", args...)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	schema = append(schema, migrations...)
	return dbutil.TrimLeadingSQLComments(schema)
}

// DatabaseExists determines whether the database exists
func (drv *Driver) DatabaseExists() (bool, error) {
	name := dbutil.DatabaseName(drv.databaseURL)

	db, err := drv.openPostgresDB()
	if err != nil {
		return false, err
	}
	defer dbutil.MustClose(db)

	exists := false
	err = db.QueryRow("select true from pg_database where datname = $1", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// MigrationsTableExists checks if the schema_migrations table exists
func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	schema, migrationsTableNameParts, err := drv.migrationsTableNameParts(db)
	if err != nil {
		return false, err
	}

	migrationsTable := strings.Join(migrationsTableNameParts, ".")
	exists := false
	err = db.QueryRow("SELECT 1 FROM information_schema.tables "+
		"WHERE  table_schema = $1 "+
		"AND    table_name   = $2",
		schema, migrationsTable).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	schema, migrationsTable, err := drv.quotedMigrationsTableNameParts(db)
	if err != nil {
		return err
	}

	// first attempt at creating migrations table
	createTableStmt := fmt.Sprintf(
		"create table if not exists %s.%s (version varchar primary key)",
		schema, migrationsTable)
	_, err = db.Exec(createTableStmt)
	if err == nil {
		// table exists or created successfully
		return nil
	}

	// catch 'schema does not exist' error
	pqErr, ok := err.(*pq.Error)
	if !ok || pqErr.Code != "3F000" {
		// unknown error
		return err
	}

	// in theory we could attempt to create the schema every time, but we avoid that
	// in case the user doesn't have permissions to create schemas
	fmt.Fprintf(drv.log, "Creating schema: %s\n", schema)
	_, err = db.Exec(fmt.Sprintf("create schema if not exists %s", schema))
	if err != nil {
		return err
	}

	// second and final attempt at creating migrations table
	_, err = db.Exec(createTableStmt)
	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return nil, err
	}

	query := "select version from " + migrationsTable + " order by version desc"
	if limit >= 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}

	defer dbutil.MustClose(rows)

	migrations := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = true
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv *Driver) InsertMigration(db dbutil.Transaction, version string) error {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("insert into "+migrationsTable+" (version) values ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("delete from "+migrationsTable+" where version = $1", version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *Driver) Ping() error {
	// attempt connection to primary database, not "postgres" database
	// to support servers with no "postgres" database
	// (see https://github.com/amacneil/dbmate/issues/78)
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	err = db.Ping()
	if err == nil {
		return nil
	}

	// ignore 'database does not exist' error
	pqErr, ok := err.(*pq.Error)
	if ok && pqErr.Code == "3D000" {
		return nil
	}

	return err
}

// Return a normalized version of the driver-specific error type.
func (drv *Driver) QueryError(query string, err error) error {
	position := 0

	if pqErr, ok := err.(*pq.Error); ok {
		if pos, err := strconv.Atoi(pqErr.Position); err == nil {
			position = pos
		}
	}

	return &dbmate.QueryError{Err: err, Query: query, Position: position}
}

func (drv *Driver) quotedMigrationsTableName(db dbutil.Transaction) (string, error) {
	schema, name, err := drv.quotedMigrationsTableNameParts(db)
	if err != nil {
		return "", err
	}

	return schema + "." + name, nil
}

func (drv *Driver) migrationsTableNameParts(db dbutil.Transaction) (string, []string, error) {
	schema := ""
	tableNameParts := strings.Split(drv.migrationsTableName, ".")
	if len(tableNameParts) > 1 {
		// schema specified as part of table name
		schema, tableNameParts = tableNameParts[0], tableNameParts[1:]
	}

	if schema == "" {
		// no schema specified with table name, try URL search path if available
		searchPath := strings.Split(drv.databaseURL.Query().Get("search_path"), ",")
		schema = strings.TrimSpace(searchPath[0])
	}

	var err error
	if schema == "" {
		// if no URL available, use current schema
		// this is a hack because we don't always have the URL context available
		schema, err = dbutil.QueryValue(db, "select current_schema()")
		if err != nil {
			return "", nil, err
		}
	}

	// fall back to public schema as last resort
	if schema == "" {
		schema = "public"
	}

	return schema, tableNameParts, nil
}

func (drv *Driver) quotedMigrationsTableNameParts(db dbutil.Transaction) (string, string, error) {
	schema, tableNameParts, err := drv.migrationsTableNameParts(db)

	if err != nil {
		return "", "", err
	}

	// Quote identifiers for Redshift and Spanner
	if drv.databaseURL.Scheme == "redshift" || drv.databaseURL.Scheme == "spanner-postgres" {
		return pq.QuoteIdentifier(schema), pq.QuoteIdentifier(strings.Join(tableNameParts, ".")), nil
	}

	// quote all parts
	// use server rather than client to do this to avoid unnecessary quotes
	// (which would change schema.sql diff)
	tableNameParts = append([]string{schema}, tableNameParts...)
	quotedNameParts, err := dbutil.QueryColumn(db, "select quote_ident(unnest($1::text[]))", pq.Array(tableNameParts))
	if err != nil {
		return "", "", err
	}

	// if more than one part, we already have a schema
	return quotedNameParts[0], strings.Join(quotedNameParts[1:], "."), nil
}
