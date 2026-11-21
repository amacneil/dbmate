package dbmate_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

var errCloseFailed = errors.New("tls: failed to send closeNotify alert (but connection was closed anyway)")

// closeFailConn is a sqlite connection whose Close always fails after closing,
// like a TLS connection the server has already dropped (#898)
type closeFailConn struct{ *sqlite3.SQLiteConn }

func (c closeFailConn) Close() error {
	_ = c.SQLiteConn.Close()
	return errCloseFailed
}

type closeFailConnector struct{ dsn string }

func (c closeFailConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite3.SQLiteDriver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return closeFailConn{conn.(*sqlite3.SQLiteConn)}, nil
}

func (c closeFailConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

// closeFailDriver is the sqlite driver, except that the connections it opens
// fail to close
type closeFailDriver struct {
	dbmate.Driver
	dsn string
}

func (d closeFailDriver) Open() (*sql.DB, error) {
	return sql.OpenDB(closeFailConnector{dsn: d.dsn}), nil
}

func init() {
	dbmate.RegisterDriver(func(config dbmate.DriverConfig) dbmate.Driver {
		u := *config.DatabaseURL
		u.Scheme = "sqlite"
		config.DatabaseURL = &u
		return closeFailDriver{Driver: sqlite.NewDriver(config), dsn: sqlite.ConnectionString(&u)}
	}, "closefail")
}

func newCloseFailDB(t *testing.T, migration string) (*dbmate.DB, *url.URL) {
	path := filepath.Join(t.TempDir(), "closefail.sqlite3")
	db := dbmate.New(dbtest.MustParseURL(t, "closefail:"+path))
	db.AutoDumpSchema = false
	db.FS = fstest.MapFS{
		"db/migrations/001_test.sql": {Data: []byte(migration)},
	}
	return db, dbtest.MustParseURL(t, "sqlite:"+path)
}

func TestMigrateCloseError(t *testing.T) {
	t.Run("migration succeeds", func(t *testing.T) {
		db, sqliteURL := newCloseFailDB(t, "-- migrate:up\ncreate table users (id integer);\n-- migrate:down\ndrop table users;\n")

		// a failed close after a successful migration is not an error
		require.NotPanics(t, func() {
			require.NoError(t, db.Migrate())
		})

		// and the migration was applied
		sqlDB, err := sql.Open("sqlite3", sqlite.ConnectionString(sqliteURL))
		require.NoError(t, err)
		defer dbutil.MustClose(sqlDB)
		count, err := dbutil.QueryValue(sqlDB, "select count(*) from schema_migrations where version = '001'")
		require.NoError(t, err)
		require.Equal(t, "1", count)
	})

	t.Run("migration fails", func(t *testing.T) {
		db, _ := newCloseFailDB(t, "-- migrate:up\nnot valid sql;\n-- migrate:down\n")

		// the migration's own error is returned, not hidden by the close error
		var err error
		require.NotPanics(t, func() {
			err = db.Migrate()
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, errCloseFailed)
		require.Contains(t, err.Error(), "syntax error")
	})
}
