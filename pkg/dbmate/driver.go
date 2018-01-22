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
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(Transaction, string) error
	DeleteMigration(Transaction, string) error
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// GetDriver loads a database driver by name
func GetDriver(name string) (Driver, error) {
	switch name {
	case "mysql":
		return MySQLDriver{}, nil
	case "postgres", "postgresql":
		return PostgresDriver{}, nil
	case "sqlite", "sqlite3":
		return SQLiteDriver{}, nil
	default:
		return nil, fmt.Errorf("unknown driver: %s", name)
	}
}

// GetDriverOpen is a shortcut for GetDriver(u.Scheme).Open(u)
func GetDriverOpen(u *url.URL) (*sql.DB, error) {
	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return nil, err
	}

	return drv.Open(u)
}
