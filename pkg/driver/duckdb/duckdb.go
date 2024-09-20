//go:build cgo
// +build cgo

// This driver is HEAVILY based on the sqlite driver
// Even many of the comments are applicable.

// TODO Features:
// - Add support for schema names, sqlite base implementation doesn't have them, duckdb does.
// 		- See postgres driver for how to do this.
// - Ensure support of non-table objects (views, macros, etc.)

package duckdb

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/lib/pq"
	_ "github.com/marcboeker/go-duckdb" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(NewDriver, "duckdb")
}

type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
}

func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
	}
}

// ConnectionString converts a URL into a valid connection string
func ConnectionString(u *url.URL) string {
	// duplicate URL and remove scheme
	newURL := *u
	newURL.Scheme = ""

	if newURL.Opaque == "" && newURL.Path != "" {
		// When the DSN is in the form "scheme:/absolute/path" or
		// "scheme://absolute/path" or "scheme:///absolute/path", url.Parse
		// will consider the file path as :
		// - "absolute" as the hostname
		// - "path" (and the rest until "?") as the URL path.
		// Instead, when the DSN is in the form "scheme:", the (relative) file
		// path is stored in the "Opaque" field.
		// See: https://pkg.go.dev/net/url#URL
		//
		// While Opaque is not escaped, the URL Path is. So, if .Path contains
		// the file path, we need to un-escape it, and rebuild the full path.

		newURL.Opaque = "//" + newURL.Host + dbutil.MustUnescapePath(newURL.Path)
		newURL.Path = ""
	}
	// trim duplicate leading slashes
	str := regexp.MustCompile("^//+").ReplaceAllString(newURL.String(), "/")

	return str
}

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("duckdb", ConnectionString(drv.databaseURL))
}

func (drv *Driver) CreateDatabase() error {
	fmt.Fprintf(drv.log, "Creating: %s\n", ConnectionString(drv.databaseURL))

	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	return db.Ping()
}

// DropDatabase drops the specified database (if it exists)
func (drv *Driver) DropDatabase() error {
	path := ConnectionString(drv.databaseURL)
	fmt.Fprintf(drv.log, "Dropping: %s\n", path)

	exists, err := drv.DatabaseExists()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	return os.Remove(path)
}

func (drv *Driver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := dbutil.QueryColumn(db,
		fmt.Sprintf("select format('\"{}\"', version) from %s order by version asc", migrationsTable))
	if err != nil {
		return nil, err
	}

	// build schema migrations table data
	var buf bytes.Buffer
	buf.WriteString("-- Dbmate schema migrations\n")

	if len(migrations) > 0 {
		buf.WriteString(
			fmt.Sprintf("INSERT INTO %s (version) VALUES\n  (", migrationsTable) +
				strings.Join(migrations, "),\n  (") +
				");\n")
	}

	return buf.Bytes(), nil
}

// DumpSchema returns the current database schema
func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
	queryString := `SELECT sql FROM (
	SELECT COALESCE(sql, format('CREATE SCHEMA {}', schema_name)) AS sql from duckdb_schemas() where internal=false
	UNION ALL
	SELECT sql from duckdb_sequences()
	UNION ALL
	SELECT sql from duckdb_tables() where internal=false
	UNION ALL
	SELECT sql from duckdb_indexes()
	UNION ALL
	SELECT sql from duckdb_views() WHERE internal=false AND sql is not null
	UNION ALL
	SELECT macro_definition from duckdb_functions() WHERE internal=false and macro_definition is not null
	) WHERE sql IS NOT NULL;
	`
	rows, err := db.Query(queryString)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schema []byte

	// Iterate over the rows and build the schema
	for rows.Next() {
		var sqlStmt string
		if err := rows.Scan(&sqlStmt); err != nil {
			return nil, err
		}
		// Append each SQL statement to the schema slice
		schema = append(schema, []byte(sqlStmt+"\n")...)
	}

	// Check for any errors encountered during iteration
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Add any migrations to the schema
	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	// Append the migrations to the schema
	schema = append(schema, migrations...)

	// Trim leading comments or unnecessary lines from the schema
	return dbutil.TrimLeadingSQLComments(schema)
}

// DatabaseExists determines whether the database exists
func (drv *Driver) DatabaseExists() (bool, error) {
	_, err := os.Stat(ConnectionString(drv.databaseURL))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// MigrationsTableExists checks if the schema_migrations table exists
func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	exists := false
	// TODO: Change this query. duckdb supports schemas and tables.
	// May need to look at another drive to see how they handle this.
	err := db.QueryRow("SELECT 1 FROM sqlite_master "+
		"WHERE type='table' AND name=$1",
		drv.migrationsTableName).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema migrations table
func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf(
		"create table if not exists %s (version varchar(128) primary key)",
		drv.quotedMigrationsTableName()))

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s order by version desc", drv.quotedMigrationsTableName())
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
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version) values (?)", drv.quotedMigrationsTableName()),
		version)

	return err
}

// DeleteMigration removes a migration record
func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("delete from %s where version = ?", drv.quotedMigrationsTableName()),
		version)

	return err
}

// Ping verifies a connection to the database. Due to the way DuckDB works, by
// testing whether the database is valid, it will automatically create the database
// if it does not already exist.
func (drv *Driver) Ping() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	return db.Ping()
}

// Return a normalized version of the driver-specific error type.
func (drv *Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}

func (drv *Driver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}

// quoteIdentifier quotes a table or column name
// we fall back to lib/pq implementation since both use ansi standard (double quotes)
// and mattn/go-duckdb doesn't provide a sqlite-specific equivalent
func (drv *Driver) quoteIdentifier(s string) string {
	return pq.QuoteIdentifier(s)
}
