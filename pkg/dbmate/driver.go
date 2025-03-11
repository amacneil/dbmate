package dbmate

import (
	"database/sql"
	"fmt"
	"io"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

// Driver provides top level database functions
type Driver interface {
	Open() (*sql.DB, error)
	DatabaseExists() (bool, error)
	CreateDatabase() error
	DropDatabase() error
	DumpSchema(*sql.DB) ([]byte, error)
	MigrationsTableExists(*sql.DB) (bool, error)
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(dbutil.Transaction, string, string) error
	DeleteMigration(dbutil.Transaction, string) error
	Ping() error
	QueryError(string, error) error
}

// DriverConfig holds configuration passed to driver constructors
type DriverConfig struct {
	DatabaseURL         *url.URL
	Log                 io.Writer
	MigrationsTableName string
}

// DriverFunc represents a driver constructor
type DriverFunc func(DriverConfig) Driver

type QueryError struct {
	Err      error
	Query    string
	Position int
}

func (e *QueryError) Error() string {
	if e.Position > 0 {
		line := 1
		column := 1
		offset := 0
		for _, ch := range e.Query {
			offset++
			if offset >= e.Position {
				break
			}
			// don't count CR as a column in CR/LF sequences
			if ch == '\r' {
				continue
			}
			if ch == '\n' {
				line++
				column = 1
				continue
			}
			column++
		}
		return fmt.Sprintf("line: %d, column: %d, position: %d: %s", line, column, e.Position, e.Err.Error())
	}

	return e.Err.Error()
}

var drivers = map[string]DriverFunc{}

// RegisterDriver registers a driver constructor for a given URL scheme
func RegisterDriver(f DriverFunc, scheme string) {
	drivers[scheme] = f
}
