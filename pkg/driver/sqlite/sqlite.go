//go:build cgo
// +build cgo

package sqlite

import (
	"database/sql"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite/internal"

	_ "github.com/mattn/go-sqlite3" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(NewDriver, "sqlite")
	dbmate.RegisterDriver(NewDriver, "sqlite3")
}

// Driver provides top level database functions
type Driver struct {
	internal *internal.Driver
}

// NewDriver initializes the driver
func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		internal: internal.NewDriver("sqlite3")(config),
	}
}

// ConnectionString converts a URL into a valid connection string
func ConnectionString(u *url.URL) string {
	return internal.ConnectionString(u)
}

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return drv.internal.Open()
}

// CreateDatabase creates the specified database
func (drv *Driver) CreateDatabase() error {
	return drv.internal.CreateDatabase()
}

// DropDatabase drops the specified database (if it exists)
func (drv *Driver) DropDatabase() error {
	return drv.internal.DropDatabase()
}

// DumpSchema returns the current database schema
func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
	return drv.internal.DumpSchema(db)
}

// DatabaseExists determines whether the database exists
func (drv *Driver) DatabaseExists() (bool, error) {
	return drv.internal.DatabaseExists()
}

// MigrationsTableExists checks if the schema_migrations table exists
func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	return drv.internal.MigrationsTableExists(db)
}

// CreateMigrationsTable creates the schema migrations table
func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	return drv.internal.CreateMigrationsTable(db)
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	return drv.internal.SelectMigrations(db, limit)
}

// InsertMigration adds a new migration record
func (drv *Driver) InsertMigration(db dbutil.Transaction, version string) error {
	return drv.internal.InsertMigration(db, version)
}

// DeleteMigration removes a migration record
func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	return drv.internal.DeleteMigration(db, version)
}

// Ping verifies a connection to the database. Due to the way SQLite works, by
// testing whether the database is valid, it will automatically create the database
// if it does not already exist.
func (drv *Driver) Ping() error {
	return drv.internal.Ping()
}

// Return a normalized version of the driver-specific error type.
func (drv *Driver) QueryError(query string, err error) error {
	return drv.internal.QueryError(query, err)
}
