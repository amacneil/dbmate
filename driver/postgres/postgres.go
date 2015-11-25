package postgres

import (
	"database/sql"
	"fmt"
	"github.com/adrianmacneil/dbmate/driver/shared"
	pq "github.com/lib/pq"
	"net/url"
)

// Driver provides top level database functions
type Driver struct {
}

// Open creates a new database connection
func (postgres Driver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("postgres", u.String())
}

// postgresExec runs a sql statement on the "postgres" database
func (postgres Driver) postgresExec(u *url.URL, statement string) error {
	// connect to postgres database
	postgresURL := *u
	postgresURL.Path = "postgres"

	db, err := postgres.Open(&postgresURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// run statement
	_, err = db.Exec(statement)

	return err
}

// CreateDatabase creates the specified database
func (postgres Driver) CreateDatabase(u *url.URL) error {
	database := shared.DatabaseName(u)
	fmt.Printf("Creating: %s\n", database)

	return postgres.postgresExec(u, fmt.Sprintf("CREATE DATABASE %s",
		pq.QuoteIdentifier(database)))
}

// DropDatabase drops the specified database (if it exists)
func (postgres Driver) DropDatabase(u *url.URL) error {
	database := shared.DatabaseName(u)
	fmt.Printf("Dropping: %s\n", database)

	return postgres.postgresExec(u, fmt.Sprintf("DROP DATABASE IF EXISTS %s",
		pq.QuoteIdentifier(database)))
}

// HasMigrationsTable returns true if the schema_migrations table exists
func (postgres Driver) HasMigrationsTable(db *sql.DB) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

// CreateMigrationsTable creates the schema_migrations table
func (postgres Driver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version varchar(255) PRIMARY KEY)`)

	return err
}

// SelectMigrations returns a list of applied migrations
func (postgres Driver) SelectMigrations(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	migrations := map[string]struct{}{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = struct{}{}
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (postgres Driver) InsertMigration(db shared.Transaction, version string) error {
	_, err := db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (postgres Driver) DeleteMigration(db shared.Transaction, version string) error {
	_, err := db.Exec("DELETE FROM schema_migrations WHERE version = $1", version)

	return err
}
