package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"
)

// Driver provides top level database functions
type Driver interface {
	Open(*url.URL) (*sql.DB, error)
	DatabaseExists(*url.URL) (bool, error)
	CreateDatabase(*url.URL) error
	DropDatabase(*url.URL) error
	DumpSchema(*url.URL, *sql.DB) ([]byte, error)
	SetMigrationsTableName(string)
	CreateMigrationsTable(*url.URL, *sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(Transaction, string) error
	DeleteMigration(Transaction, string) error
	Ping(*url.URL) error
}

var drivers = map[string]Driver{}

// RegisterDriver registers a driver for a URL scheme
func RegisterDriver(drv Driver, scheme string) {
	drivers[scheme] = drv
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// getDriver loads a database driver by name
func getDriver(name string) (Driver, error) {
	if drv, ok := drivers[name]; ok {
		drv.SetMigrationsTableName(DefaultMigrationsTableName)

		return drv, nil
	}

	return nil, fmt.Errorf("unsupported driver: %s", name)
}

// getDriverOpen is a shortcut for GetDriver(u.Scheme).Open(u)
func getDriverOpen(u *url.URL) (*sql.DB, error) {
	drv, err := getDriver(u.Scheme)
	if err != nil {
		return nil, err
	}

	return drv.Open(u)
}
