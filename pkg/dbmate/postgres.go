package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/lib/pq"
)

// PostgresDriver provides top level database functions
type PostgresDriver struct {
}

// Open creates a new database connection
func (drv PostgresDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("postgres", u.String())
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

// DumpSchema returns the current database schema
func (drv PostgresDriver) DumpSchema(u *url.URL) ([]byte, error) {
	return runCommand("pg_dump", "--format=plain", "--encoding=UTF8", "--schema-only",
		"--no-acl", "--no-owner", u.String())
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
func (drv PostgresDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists schema_migrations (
		version varchar(255) primary key)`)

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv PostgresDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
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
func (drv PostgresDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv PostgresDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = $1", version)

	return err
}
