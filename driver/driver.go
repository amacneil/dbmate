package driver

import (
	"database/sql"
	"fmt"
	"github.com/adrianmacneil/dbmate/driver/mysql"
	"github.com/adrianmacneil/dbmate/driver/postgres"
	"github.com/adrianmacneil/dbmate/driver/shared"
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
	InsertMigration(shared.Transaction, string) error
	DeleteMigration(shared.Transaction, string) error
}

// Get loads a database driver by name
func Get(name string) (Driver, error) {
	switch name {
	case "mysql":
		return mysql.Driver{}, nil
	case "postgres":
		return postgres.Driver{}, nil
	default:
		return nil, fmt.Errorf("Unknown driver: %s", name)
	}
}

// Open is a shortcut for driver.Get(u.Scheme).Open(u)
func Open(u *url.URL) (*sql.DB, error) {
	drv, err := Get(u.Scheme)
	if err != nil {
		return nil, err
	}

	return drv.Open(u)
}
