package dbmate_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
	"github.com/amacneil/dbmate/v2/pkg/driver/postgres"

	"github.com/stretchr/testify/require"
)

func TestPostgresApplicationNameFormatting(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		version  string
		expected string
	}{
		{
			name:     "empty base with version",
			base:     "",
			version:  "20251111103123",
			expected: "dbmate[20251111103123]",
		},
		{
			name:     "dbmate base with version",
			base:     "dbmate",
			version:  "20251111103123",
			expected: "dbmate[20251111103123]",
		},
		{
			name:     "custom base with version",
			base:     "my-service",
			version:  "20251111103123",
			expected: "my-service[20251111103123]",
		},
		{
			name:     "empty base and empty version",
			base:     "",
			version:  "",
			expected: "dbmate",
		},
		{
			name:     "custom base and empty version",
			base:     "my-service",
			version:  "",
			expected: "my-service",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := postgres.FormatApplicationName(c.base, c.version)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestPostgresConnectionConfiguration(t *testing.T) {
	t.Run("default URL configures dbmate with migration version", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "postgres://localhost/db")
		configured := postgres.ConnectionURLWithApplicationName(u, "20251111103123")
		require.Equal(t, "dbmate[20251111103123]", configured.Query().Get("application_name"))
	})

	t.Run("URL with custom application_name appends migration version", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "postgres://localhost/db?application_name=reporting_api")
		configured := postgres.ConnectionURLWithApplicationName(u, "20251111103123")
		require.Equal(t, "reporting_api[20251111103123]", configured.Query().Get("application_name"))
	})

	t.Run("driver application name resolution", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "postgres://localhost/db"))
		drvInterface, err := db.Driver()
		require.NoError(t, err)

		drv, ok := drvInterface.(*postgres.Driver)
		require.True(t, ok)
		require.Equal(t, "dbmate[20251111103123]", drv.ApplicationName("20251111103123"))
	})

	t.Run("driver custom application name resolution", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "postgres://localhost/db?application_name=custom_worker"))
		drvInterface, err := db.Driver()
		require.NoError(t, err)

		drv, ok := drvInterface.(*postgres.Driver)
		require.True(t, ok)
		require.Equal(t, "custom_worker[20251111103123]", drv.ApplicationName("20251111103123"))
	})
}

type recordingTx struct {
	executed []string
}

func (r *recordingTx) Exec(query string, _ ...interface{}) (sql.Result, error) {
	r.executed = append(r.executed, query)
	return nil, nil
}

func (r *recordingTx) Query(_ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (r *recordingTx) QueryRow(_ string, _ ...interface{}) *sql.Row {
	return nil
}

func TestPostgresSetMigrationContextLogic(t *testing.T) {
	t.Run("formats set application_name statement", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "postgres://localhost/db"))
		drvInterface, err := db.Driver()
		require.NoError(t, err)

		ctxDrv, ok := drvInterface.(dbmate.DriverMigrationContext)
		require.True(t, ok)

		tx := &recordingTx{}
		err = ctxDrv.SetMigrationContext(tx, "20251111103123")
		require.NoError(t, err)
		require.Equal(t, []string{"set application_name = 'dbmate[20251111103123]'"}, tx.executed)
	})

	t.Run("formats set application_name statement with custom name", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "postgres://localhost/db?application_name=my_app"))
		drvInterface, err := db.Driver()
		require.NoError(t, err)

		ctxDrv, ok := drvInterface.(dbmate.DriverMigrationContext)
		require.True(t, ok)

		tx := &recordingTx{}
		err = ctxDrv.SetMigrationContext(tx, "20251111103123")
		require.NoError(t, err)
		require.Equal(t, []string{"set application_name = 'my_app[20251111103123]'"}, tx.executed)
	})
}

type contextTrackingDriver struct {
	dbmate.Driver
	migrationContexts []string
}

func (d *contextTrackingDriver) SetMigrationContext(_ dbutil.Transaction, version string) error {
	d.migrationContexts = append(d.migrationContexts, version)
	return nil
}

func TestPostgresMigrateInvocationOfDriverMigrationContext(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_context.sqlite3")
	testURL := dbtest.MustParseURL(t, "sqlite:"+dbPath)
	db := newTestDB(t, testURL)

	baseDrv, err := db.Driver()
	require.NoError(t, err)

	trackingDrv := &contextTrackingDriver{Driver: baseDrv}

	customScheme := "test-context-postgres"
	dbmate.RegisterDriver(func(_ dbmate.DriverConfig) dbmate.Driver {
		return trackingDrv
	}, customScheme)

	db.DatabaseURL = dbtest.MustParseURL(t, customScheme+"://test")
	db.DriverName = customScheme

	sqlDB, err := baseDrv.Open()
	require.NoError(t, err)
	defer dbutil.MustClose(sqlDB)

	err = baseDrv.CreateMigrationsTable(sqlDB)
	require.NoError(t, err)

	db.FS = fstest.MapFS{
		"db/migrations/20251111103123_first.sql": {
			Data: []byte("-- migrate:up\ncreate table t1 (id int);\n-- migrate:down\ndrop table t1;\n"),
		},
		"db/migrations/20251111103124_second.sql": {
			Data: []byte("-- migrate:up\ncreate table t2 (id int);\n-- migrate:down\ndrop table t2;\n"),
		},
	}

	err = db.Migrate()
	require.NoError(t, err)
	require.Equal(t, []string{"20251111103123", "20251111103124"}, trackingDrv.migrationContexts)

	trackingDrv.migrationContexts = nil
	err = db.Rollback()
	require.NoError(t, err)
	require.Equal(t, []string{"20251111103124"}, trackingDrv.migrationContexts)
}
