package dbmate

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"
)

func init() {
	RegisterDriver(newPostgresDriver, "postgres")
	RegisterDriver(newPostgresDriver, "postgresql")
}

// PostgresDriver provides top level database functions
type PostgresDriver struct {
	migrationsTableName string
	databaseURL         *url.URL
}

func newPostgresDriver(config DriverConfig) Driver {
	return &PostgresDriver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
	}
}

func normalizePostgresURL(u *url.URL) *url.URL {
	hostname := u.Hostname()
	port := u.Port()
	query := u.Query()

	// support socket parameter for consistency with mysql
	if query.Get("socket") != "" {
		query.Set("host", query.Get("socket"))
		query.Del("socket")
	}

	// default hostname
	if hostname == "" {
		hostname = "localhost"
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
		port = "5432"
	}

	// generate output URL
	out, _ := url.Parse(u.String())
	out.Host = fmt.Sprintf("%s:%s", hostname, port)
	out.RawQuery = query.Encode()

	return out
}

func normalizePostgresURLForDump(u *url.URL) []string {
	u = normalizePostgresURL(u)

	// find schemas from search_path
	query := u.Query()
	schemas := strings.Split(query.Get("search_path"), ",")
	query.Del("search_path")
	u.RawQuery = query.Encode()

	out := []string{}
	for _, schema := range schemas {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			out = append(out, "--schema", schema)
		}
	}
	out = append(out, u.String())

	return out
}

// Open creates a new database connection
func (drv *PostgresDriver) Open() (*sql.DB, error) {
	return sql.Open("postgres", normalizePostgresURL(drv.databaseURL).String())
}

func (drv *PostgresDriver) openPostgresDB() (*sql.DB, error) {
	// clone databaseURL
	postgresURL, err := url.Parse(normalizePostgresURL(drv.databaseURL).String())
	if err != nil {
		return nil, err
	}

	// connect to postgres database
	postgresURL.Path = "postgres"

	return sql.Open("postgres", postgresURL.String())
}

// CreateDatabase creates the specified database
func (drv *PostgresDriver) CreateDatabase() error {
	name := databaseName(drv.databaseURL)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openPostgresDB()
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *PostgresDriver) DropDatabase() error {
	name := databaseName(drv.databaseURL)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openPostgresDB()
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		pq.QuoteIdentifier(name)))

	return err
}

func (drv *PostgresDriver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return nil, err
	}

	// load applied migrations
	migrations, err := queryColumn(db,
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
func (drv *PostgresDriver) DumpSchema(db *sql.DB) ([]byte, error) {
	// load schema
	args := append([]string{"--format=plain", "--encoding=UTF8", "--schema-only",
		"--no-privileges", "--no-owner"}, normalizePostgresURLForDump(drv.databaseURL)...)
	schema, err := runCommand("pg_dump", args...)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	schema = append(schema, migrations...)
	return trimLeadingSQLComments(schema)
}

// DatabaseExists determines whether the database exists
func (drv *PostgresDriver) DatabaseExists() (bool, error) {
	name := databaseName(drv.databaseURL)

	db, err := drv.openPostgresDB()
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow("select true from pg_database where datname = $1", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv *PostgresDriver) CreateMigrationsTable(db *sql.DB) error {
	schema, migrationsTable, err := drv.quotedMigrationsTableNameParts(db)
	if err != nil {
		return err
	}

	// first attempt at creating migrations table
	createTableStmt := fmt.Sprintf("create table if not exists %s.%s", schema, migrationsTable) +
		" (version varchar(255) primary key)"
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
	fmt.Printf("Creating schema: %s\n", schema)
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
func (drv *PostgresDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
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

	defer mustClose(rows)

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
func (drv *PostgresDriver) InsertMigration(db Transaction, version string) error {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("insert into "+migrationsTable+" (version) values ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv *PostgresDriver) DeleteMigration(db Transaction, version string) error {
	migrationsTable, err := drv.quotedMigrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("delete from "+migrationsTable+" where version = $1", version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *PostgresDriver) Ping() error {
	// attempt connection to primary database, not "postgres" database
	// to support servers with no "postgres" database
	// (see https://github.com/amacneil/dbmate/issues/78)
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer mustClose(db)

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

func (drv *PostgresDriver) quotedMigrationsTableName(db Transaction) (string, error) {
	schema, name, err := drv.quotedMigrationsTableNameParts(db)
	if err != nil {
		return "", err
	}

	return schema + "." + name, nil
}

func (drv *PostgresDriver) quotedMigrationsTableNameParts(db Transaction) (string, string, error) {
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
		schema, err = queryValue(db, "select current_schema()")
		if err != nil {
			return "", "", err
		}
	}

	// fall back to public schema as last resort
	if schema == "" {
		schema = "public"
	}

	// quote all parts
	// use server rather than client to do this to avoid unnecessary quotes
	// (which would change schema.sql diff)
	tableNameParts = append([]string{schema}, tableNameParts...)
	quotedNameParts, err := queryColumn(db, "select quote_ident(unnest($1::text[]))", pq.Array(tableNameParts))
	if err != nil {
		return "", "", err
	}

	// if more than one part, we already have a schema
	return quotedNameParts[0], strings.Join(quotedNameParts[1:], "."), nil
}
