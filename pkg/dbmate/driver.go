package dbmate

import (
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

// Driver provides top level database functions
type Driver interface {
	Open() (*sql.DB, error)
	DatabaseExists() (bool, error)
	CreateDatabase() error
	DropDatabase() error
	DumpSchema(*sql.DB, ...string) ([]byte, error)
	MigrationsTableExists(*sql.DB) (bool, error)
	CreateMigrationsTable(*sql.DB) error
	SelectMigrations(*sql.DB, int) (map[string]bool, error)
	InsertMigration(dbutil.Transaction, string) error
	DeleteMigration(dbutil.Transaction, string) error
	Ping() error
	QueryError(string, error) error
}

// DriverMigrationLock is implemented by drivers that can hold an exclusive lock
// for the duration of a migration run, so that concurrent dbmate instances wait
// for each other instead of racing to apply the same migrations.
type DriverMigrationLock interface {
	// OpenWithMigrationLock opens a database handle backed by a single connection
	// that holds the migration lock, waiting up to timeout for another holder to
	// release it (zero waits indefinitely).
	//
	// Closing the handle releases the lock, so a crashed dbmate never leaves a
	// stale lock behind. If the connection is lost the handle returns an error
	// from the next statement rather than silently reconnecting without the lock,
	// so that the lock and the migrations share a fate.
	OpenWithMigrationLock(timeout time.Duration) (*sql.DB, error)
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
