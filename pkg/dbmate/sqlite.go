// +build cgo

package dbmate

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3" // sqlite driver for database/sql
)

func init() {
	drv := &SQLiteDriver{}
	RegisterDriver(drv, "sqlite")
	RegisterDriver(drv, "sqlite3")
}

// SQLiteDriver provides top level database functions
type SQLiteDriver struct {
	migrationsTableName string
}

func sqlitePath(u *url.URL) string {
	// strip one leading slash
	// absolute URLs can be specified as sqlite:////tmp/foo.sqlite3
	str := regexp.MustCompile("^/").ReplaceAllString(u.Path, "")

	return str
}

// SetMigrationsTableName sets the schema migrations table name
func (drv *SQLiteDriver) SetMigrationsTableName(name string) {
	drv.migrationsTableName = name
}

// Open creates a new database connection
func (drv *SQLiteDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("sqlite3", sqlitePath(u))
}

// CreateDatabase creates the specified database
func (drv *SQLiteDriver) CreateDatabase(u *url.URL) error {
	fmt.Printf("Creating: %s\n", sqlitePath(u))

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	return db.Ping()
}

// DropDatabase drops the specified database (if it exists)
func (drv *SQLiteDriver) DropDatabase(u *url.URL) error {
	path := sqlitePath(u)
	fmt.Printf("Dropping: %s\n", path)

	exists, err := drv.DatabaseExists(u)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	return os.Remove(path)
}

func (drv *SQLiteDriver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := queryColumn(db,
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
func (drv *SQLiteDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	path := sqlitePath(u)
	schema, err := runCommand("sqlite3", path, ".schema")
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
func (drv *SQLiteDriver) DatabaseExists(u *url.URL) (bool, error) {
	_, err := os.Stat(sqlitePath(u))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateMigrationsTable creates the schema migrations table
func (drv *SQLiteDriver) CreateMigrationsTable(u *url.URL, db *sql.DB) error {
	_, err := db.Exec(
		fmt.Sprintf("create table if not exists %s ", drv.quotedMigrationsTableName()) +
			"(version varchar(255) primary key)")

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *SQLiteDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s order by version desc", drv.quotedMigrationsTableName())
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
func (drv *SQLiteDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version) values (?)", drv.quotedMigrationsTableName()),
		version)

	return err
}

// DeleteMigration removes a migration record
func (drv *SQLiteDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("delete from %s where version = ?", drv.quotedMigrationsTableName()),
		version)

	return err
}

// Ping verifies a connection to the database. Due to the way SQLite works, by
// testing whether the database is valid, it will automatically create the database
// if it does not already exist.
func (drv *SQLiteDriver) Ping(u *url.URL) error {
	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	return db.Ping()
}

func (drv *SQLiteDriver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}

// quoteIdentifier quotes a table or column name
// we fall back to lib/pq implementation since both use ansi standard (double quotes)
// and mattn/go-sqlite3 doesn't provide a sqlite-specific equivalent
func (drv *SQLiteDriver) quoteIdentifier(s string) string {
	return pq.QuoteIdentifier(s)
}
