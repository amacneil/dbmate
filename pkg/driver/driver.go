package driver

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
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(Transaction, string) error
	DeleteMigration(Transaction, string) error
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

var Drivers map[string]Driver

func init() {
	Drivers = make(map[string]Driver)
}

func Register(name string, driver Driver) {
	Drivers[name] = driver
}

// GetDriver loads a database driver by name
func GetDriver(name string) (Driver, error) {
	driver, ok := Drivers[name]
	if !ok {
		return nil, fmt.Errorf("Unknown driver: %s", name)
	}
	return driver, nil
}

// GetDriverOpen is a shortcut for GetDriver(u.Scheme).Open(u)
func GetDriverOpen(u *url.URL) (*sql.DB, error) {
	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return nil, err
	}

	return drv.Open(u)
}
