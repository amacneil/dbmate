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
	RegisterDriver(PostgresDriver{}, "postgres")
	RegisterDriver(PostgresDriver{}, "postgresql")
}

// PostgresDriver provides top level database functions
type PostgresDriver struct {
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
func (drv PostgresDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("postgres", normalizePostgresURL(u).String())
}

func (drv PostgresDriver) openPostgresDB(u *url.URL) (*sql.DB, error) {
	// connect to postgres database
	postgresURL := *u
	postgresURL.Path = "postgres"

	return drv.Open(&postgresURL)
}

// CreateDatabase creates the specified database
func (drv PostgresDriver) CreateDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv PostgresDriver) DropDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		pq.QuoteIdentifier(name)))

	return err
}

func (drv PostgresDriver) postgresSchemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable, err := drv.migrationsTableName(db)
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
func (drv PostgresDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	// load schema
	args := append([]string{"--format=plain", "--encoding=UTF8", "--schema-only",
		"--no-privileges", "--no-owner"}, normalizePostgresURLForDump(u)...)
	schema, err := runCommand("pg_dump", args...)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.postgresSchemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	schema = append(schema, migrations...)
	return trimLeadingSQLComments(schema)
}

// DatabaseExists determines whether the database exists
func (drv PostgresDriver) DatabaseExists(u *url.URL) (bool, error) {
	name := databaseName(u)

	db, err := drv.openPostgresDB(u)
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
func (drv PostgresDriver) CreateMigrationsTable(u *url.URL, db *sql.DB) error {
	// get schema from URL search_path param
	searchPath := strings.Split(u.Query().Get("search_path"), ",")
	urlSchema := strings.TrimSpace(searchPath[0])
	if urlSchema == "" {
		urlSchema = "public"
	}

	// get *unquoted* current schema from database
	dbSchema, err := queryRow(db, "select current_schema()")
	if err != nil {
		return err
	}

	// if urlSchema and dbSchema are not equal, the most likely explanation is that the schema
	// has not yet been created
	if urlSchema != dbSchema {
		// in theory we could just execute this statement every time, but we do the comparison
		// above in case the user doesn't have permissions to create schemas and the schema
		// already exists
		fmt.Printf("Creating schema: %s\n", urlSchema)
		_, err = db.Exec("create schema if not exists " + pq.QuoteIdentifier(urlSchema))
		if err != nil {
			return err
		}
	}

	migrationsTable, err := drv.migrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("create table if not exists " + migrationsTable +
		" (version varchar(255) primary key)")

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv PostgresDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	migrationsTable, err := drv.migrationsTableName(db)
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
func (drv PostgresDriver) InsertMigration(db Transaction, version string) error {
	migrationsTable, err := drv.migrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("insert into "+migrationsTable+" (version) values ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv PostgresDriver) DeleteMigration(db Transaction, version string) error {
	migrationsTable, err := drv.migrationsTableName(db)
	if err != nil {
		return err
	}

	_, err = db.Exec("delete from "+migrationsTable+" where version = $1", version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv PostgresDriver) Ping(u *url.URL) error {
	// attempt connection to primary database, not "postgres" database
	// to support servers with no "postgres" database
	// (see https://github.com/amacneil/dbmate/issues/78)
	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	err = db.Ping()
	if err == nil {
		return nil
	}

	// ignore 'database "foo" does not exist' error
	pqErr, ok := err.(*pq.Error)
	if ok && pqErr.Code == "3D000" {
		return nil
	}

	return err
}

func (drv PostgresDriver) migrationsTableName(db Transaction) (string, error) {
	// get current schema
	schema, err := queryRow(db, "select quote_ident(current_schema())")
	if err != nil {
		return "", err
	}

	// if the search path is empty, or does not contain a valid schema, default to public
	if schema == "" {
		schema = "public"
	}

	return schema + ".schema_migrations", nil
}
