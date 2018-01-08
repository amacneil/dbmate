package dbmate

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"

	_ "github.com/mattn/go-sqlite3" // sqlite driver for database/sql
)

// SQLiteDriver provides top level database functions
type SQLiteDriver struct {
}

func sqlitePath(u *url.URL) string {
	// strip one leading slash
	// absolute URLs can be specified as sqlite:////tmp/foo.sqlite3
	str := regexp.MustCompile("^/").ReplaceAllString(u.Path, "")

	return str
}

// Open creates a new database connection
func (drv SQLiteDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("sqlite3", sqlitePath(u))
}

// CreateDatabase creates the specified database
func (drv SQLiteDriver) CreateDatabase(u *url.URL) error {
	fmt.Printf("Creating: %s\n", sqlitePath(u))

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	return db.Ping()
}

// DropDatabase drops the specified database (if it exists)
func (drv SQLiteDriver) DropDatabase(u *url.URL) error {
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

// DumpSchema writes the current database schema to a file
func (drv SQLiteDriver) DumpSchema(u *url.URL) error {
	return errors.New("not implemented")
}

// DatabaseExists determines whether the database exists
func (drv SQLiteDriver) DatabaseExists(u *url.URL) (bool, error) {
	_, err := os.Stat(sqlitePath(u))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateMigrationsTable creates the schema_migrations table
func (drv SQLiteDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists schema_migrations (
		version varchar(255) primary key)`)

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv SQLiteDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := "select version from schema_migrations order by version desc"
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

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv SQLiteDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values (?)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv SQLiteDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = ?", version)

	return err
}
