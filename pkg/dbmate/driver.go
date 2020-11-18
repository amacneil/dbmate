package dbmate

import (
	"database/sql"
	"net/url"
)

// Driver provides top level database functions
type Driver interface {
	Open() (*sql.DB, error)
	DatabaseExists() (bool, error)
	CreateDatabase() error
	DropDatabase() error
	DumpSchema(*sql.DB) ([]byte, error)
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(Transaction, string) error
	DeleteMigration(Transaction, string) error
	Ping() error
}

// DriverConfig holds configuration passed to driver constructors
type DriverConfig struct {
	DatabaseURL         *url.URL
	MigrationsTableName string
}

// DriverFunc represents a driver constructor
type DriverFunc func(DriverConfig) Driver

var drivers = map[string]DriverFunc{}

// RegisterDriver registers a driver constructor for a given URL scheme
func RegisterDriver(f DriverFunc, scheme string) {
	drivers[scheme] = f
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}
