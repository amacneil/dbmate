//go:build cgo
// +build cgo

package sqlite

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(NewDriver, "sqlite")
	dbmate.RegisterDriver(NewDriver, "sqlite3")
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

func normalizeSQLiteURL(u *url.URL) *url.URL {
	out := &url.URL{
		Scheme:   u.Scheme,
		RawQuery: u.RawQuery,
		Fragment: u.Fragment,
	}

	var raw string
	switch {
	case u.Opaque != "":
		raw = u.Opaque
	case u.Host != "":
		raw = "/" + u.Host + u.Path
	default:
		raw = u.Path
	}

	if p, err := url.PathUnescape(raw); err == nil {
		raw = p
	}

	// Collapse runs of leading slashes: ////tmp/foo -> /tmp/foo
	for len(raw) > 1 && raw[0] == '/' && raw[1] == '/' {
		raw = raw[1:]
	}

	if filepath.IsAbs(raw) {
		// Absolute: use Path so .String() produces sqlite:///abs/path
		out.Path = filepath.ToSlash(raw)
	} else {
		// Relative: use Opaque so .String() produces sqlite:rel/path
		out.Opaque = raw
	}

	return out
}

func filePathFromURL(u *url.URL) string {
	u = normalizeSQLiteURL(u)
	if u.Opaque != "" {
		return u.Opaque
	}
	return u.Path
}

// TODO: Confirm we aren't breaking anyone's expectations and
// rename this to connectionString, as it's not part of the
// public Driver interface.
func ConnectionString(u *url.URL) string {
	u = normalizeSQLiteURL(u)
	p := filePathFromURL(u)
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		p += "#" + u.Fragment
	}
	return p
}

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("sqlite3", ConnectionString(drv.databaseURL))
}

// CreateDatabase creates the specified database
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
		fmt.Sprintf("select quote(version) from %s order by version asc", migrationsTable))
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
	path := filePathFromURL(drv.databaseURL)
	schema, err := dbutil.RunCommand("sqlite3", path, ".schema --nosys")
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
	_, err := os.Stat(filePathFromURL(drv.databaseURL))
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

// Ping verifies a connection to the database. Due to the way SQLite works, by
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
// and mattn/go-sqlite3 doesn't provide a sqlite-specific equivalent
func (drv *Driver) quoteIdentifier(s string) string {
	return pq.QuoteIdentifier(s)
}
