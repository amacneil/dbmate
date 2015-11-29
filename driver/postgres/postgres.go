package postgres

import (
	"database/sql"
	"fmt"
	"github.com/adrianmacneil/dbmate/driver/shared"
	"github.com/lib/pq"
	"io"
	"net/url"
)

// Driver provides top level database functions
type Driver struct {
}

// Open creates a new database connection
func (drv Driver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("postgres", u.String())
}

func (drv Driver) openPostgresDB(u *url.URL) (*sql.DB, error) {
	// connect to postgres database
	postgresURL := *u
	postgresURL.Path = "postgres"

	return drv.Open(&postgresURL)
}

func mustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
}

// CreateDatabase creates the specified database
func (drv Driver) CreateDatabase(u *url.URL) error {
	name := shared.DatabaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv Driver) DropDatabase(u *url.URL) error {
	name := shared.DatabaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s",
		pq.QuoteIdentifier(name)))

	return err
}

// DatabaseExists determines whether the database exists
func (drv Driver) DatabaseExists(u *url.URL) (bool, error) {
	name := shared.DatabaseName(u)

	db, err := drv.openPostgresDB(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow("SELECT true FROM pg_database WHERE datname = $1", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv Driver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version varchar(255) PRIMARY KEY)`)

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := "SELECT version FROM schema_migrations ORDER BY version DESC"
	if limit >= 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, limit)
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
func (drv Driver) InsertMigration(db shared.Transaction, version string) error {
	_, err := db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv Driver) DeleteMigration(db shared.Transaction, version string) error {
	_, err := db.Exec("DELETE FROM schema_migrations WHERE version = $1", version)

	return err
}
