package driver

import (
	"database/sql"
	"fmt"
	"github.com/adrianmacneil/dbmate/driver/postgres"
	"github.com/adrianmacneil/dbmate/driver/shared"
	"net/url"
)

// Driver provides top level database functions
type Driver interface {
	Open(*url.URL) (*sql.DB, error)
	CreateDatabase(*url.URL) error
	DropDatabase(*url.URL) error
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB) (map[string]struct{}, error)
	InsertMigration(shared.Transaction, string) error
	DeleteMigration(shared.Transaction, string) error
}

// Get loads a database driver by name
func Get(name string) (Driver, error) {
	switch name {
	case "postgres":
		return postgres.Driver{}, nil
	default:
		return nil, fmt.Errorf("Unknown driver: %s", name)
	}
}
