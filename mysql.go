package main

import (
	"database/sql"
	"fmt"
	_ "github.com/adrianmacneil/go-mysql" // mysql driver
	"net/url"
	"strings"
)

// MySQLDriver provides top level database functions
type MySQLDriver struct {
}

func normalizeMySQLURL(u *url.URL) string {
	normalizedURL := *u
	normalizedURL.Scheme = ""
	normalizedURL.Host = fmt.Sprintf("tcp(%s)", normalizedURL.Host)

	query := normalizedURL.Query()
	query.Set("multiStatements", "true")
	normalizedURL.RawQuery = query.Encode()

	str := normalizedURL.String()
	return strings.TrimLeft(str, "/")
}

// Open creates a new database connection
func (drv MySQLDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("mysql", normalizeMySQLURL(u))
}

func (drv MySQLDriver) openRootDB(u *url.URL) (*sql.DB, error) {
	// connect to no particular database
	rootURL := *u
	rootURL.Path = "/"

	return drv.Open(&rootURL)
}

func quoteIdentifier(str string) string {
	str = strings.Replace(str, "`", "\\`", -1)

	return fmt.Sprintf("`%s`", str)
}

// CreateDatabase creates the specified database
func (drv MySQLDriver) CreateDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		quoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv MySQLDriver) DropDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		quoteIdentifier(name)))

	return err
}

// DatabaseExists determines whether the database exists
func (drv MySQLDriver) DatabaseExists(u *url.URL) (bool, error) {
	name := databaseName(u)

	db, err := drv.openRootDB(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow(`select true from information_schema.schemata
		where schema_name = ?`, name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv MySQLDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists schema_migrations (
		version varchar(255) primary key)`)

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv MySQLDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
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
func (drv MySQLDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values (?)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv MySQLDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = ?", version)

	return err
}
