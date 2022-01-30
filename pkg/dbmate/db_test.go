package dbmate_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amacneil/dbmate/pkg/dbmate"
	"github.com/amacneil/dbmate/pkg/dbutil"
	_ "github.com/amacneil/dbmate/pkg/driver/mysql"
	_ "github.com/amacneil/dbmate/pkg/driver/postgres"
	_ "github.com/amacneil/dbmate/pkg/driver/sqlite"

	"github.com/kami-zh/go-capturer"
	"github.com/stretchr/testify/require"
)

var testdataDir string

func newTestDB(t *testing.T, u *url.URL) *dbmate.DB {
	var err error

	// only chdir once, because testdata is relative to current directory
	if testdataDir == "" {
		testdataDir, err = filepath.Abs("../../testdata")
		require.NoError(t, err)

		err = os.Chdir(testdataDir)
		require.NoError(t, err)
	}

	db := dbmate.New(u)
	db.AutoDumpSchema = false

	return db
}

func TestNew(t *testing.T) {
	db := dbmate.New(dbutil.MustParseURL("foo:test"))
	require.True(t, db.AutoDumpSchema)
	require.Equal(t, "foo:test", db.DatabaseURL.String())
	require.Equal(t, "./db/migrations", db.MigrationsDir)
	require.Equal(t, "schema_migrations", db.MigrationsTableName)
	require.Equal(t, "./db/schema.sql", db.SchemaFile)
	require.False(t, db.WaitBefore)
	require.Equal(t, time.Second, db.WaitInterval)
	require.Equal(t, 60*time.Second, db.WaitTimeout)
}

func TestGetDriver(t *testing.T) {
	t.Run("missing URL", func(t *testing.T) {
		db := dbmate.New(nil)
		drv, err := db.GetDriver()
		require.Nil(t, drv)
		require.EqualError(t, err, "invalid url, have you set your --url flag or DATABASE_URL environment variable?")
	})

	t.Run("missing schema", func(t *testing.T) {
		db := dbmate.New(dbutil.MustParseURL("//hi"))
		drv, err := db.GetDriver()
		require.Nil(t, drv)
		require.EqualError(t, err, "invalid url, have you set your --url flag or DATABASE_URL environment variable?")
	})

	t.Run("invalid driver", func(t *testing.T) {
		db := dbmate.New(dbutil.MustParseURL("foo://bar"))
		drv, err := db.GetDriver()
		require.EqualError(t, err, "unsupported driver: foo")
		require.Nil(t, drv)
	})
}

func TestWait(t *testing.T) {
	u := dbutil.MustParseURL(os.Getenv("POSTGRES_TEST_URL"))
	db := newTestDB(t, u)

	// speed up our retry loop for testing
	db.WaitInterval = time.Millisecond
	db.WaitTimeout = 5 * time.Millisecond

	// drop database
	err := db.Drop()
	require.NoError(t, err)

	// test wait
	err = db.Wait()
	require.NoError(t, err)

	// test invalid connection
	u.Host = "postgres:404"
	err = db.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to connect to database: dial tcp")
	require.Contains(t, err.Error(), "connect: connection refused")
}

func TestDumpSchema(t *testing.T) {
	u := dbutil.MustParseURL(os.Getenv("POSTGRES_TEST_URL"))
	db := newTestDB(t, u)

	// create custom schema file directory
	dir, err := os.MkdirTemp("", "dbmate")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(dir)
		require.NoError(t, err)
	}()

	// create schema.sql in subdirectory to test creating directory
	db.SchemaFile = filepath.Join(dir, "/schema/schema.sql")

	// drop database
	err = db.Drop()
	require.NoError(t, err)

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// dump schema
	err = db.DumpSchema()
	require.NoError(t, err)

	// verify schema
	schema, err := os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")
}

func TestAutoDumpSchema(t *testing.T) {
	u := dbutil.MustParseURL(os.Getenv("POSTGRES_TEST_URL"))
	db := newTestDB(t, u)
	db.AutoDumpSchema = true

	// create custom schema file directory
	dir, err := os.MkdirTemp("", "dbmate")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(dir)
		require.NoError(t, err)
	}()

	// create schema.sql in subdirectory to test creating directory
	db.SchemaFile = filepath.Join(dir, "/schema/schema.sql")

	// drop database
	err = db.Drop()
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)

	// verify schema
	schema, err := os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")

	// remove schema
	err = os.Remove(db.SchemaFile)
	require.NoError(t, err)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)

	// schema should be recreated
	schema, err = os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")
}

func checkWaitCalled(t *testing.T, u *url.URL, command func() error) {
	oldHost := u.Host
	u.Host = "postgres:404"
	err := command()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to connect to database: dial tcp")
	require.Contains(t, err.Error(), "connect: connection refused")
	u.Host = oldHost
}

func testWaitBefore(t *testing.T, verbose bool) {
	u := dbutil.MustParseURL(os.Getenv("POSTGRES_TEST_URL"))
	db := newTestDB(t, u)
	db.Verbose = verbose
	db.WaitBefore = true
	// so that checkWaitCalled returns quickly
	db.WaitInterval = time.Millisecond
	db.WaitTimeout = 5 * time.Millisecond

	// drop database
	err := db.Drop()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.Drop)

	// create
	err = db.Create()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.Create)

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.CreateAndMigrate)

	// migrate
	err = db.Migrate()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.Migrate)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.Rollback)

	// dump
	err = db.DumpSchema()
	require.NoError(t, err)
	checkWaitCalled(t, u, db.DumpSchema)
}

func TestWaitBefore(t *testing.T) {
	testWaitBefore(t, false)
}

func TestWaitBeforeVerbose(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		testWaitBefore(t, true)
	})
	require.Contains(t, output,
		`Applying: 20151129054053_test_migration.sql
Rows affected: 1
Applying: 20200227231541_test_posts.sql
Rows affected: 0`)
	require.Contains(t, output,
		`Rolling back: 20200227231541_test_posts.sql
Rows affected: 0`)
}

func testURLs() []*url.URL {
	return []*url.URL{
		dbutil.MustParseURL(os.Getenv("MYSQL_TEST_URL")),
		dbutil.MustParseURL(os.Getenv("POSTGRES_TEST_URL")),
		dbutil.MustParseURL(os.Getenv("SQLITE_TEST_URL")),
	}
}

func TestMigrate(t *testing.T) {
	for _, u := range testURLs() {
		t.Run(u.Scheme, func(t *testing.T) {
			db := newTestDB(t, u)
			drv, err := db.GetDriver()
			require.NoError(t, err)

			// drop and recreate database
			err = db.Drop()
			require.NoError(t, err)
			err = db.Create()
			require.NoError(t, err)

			// migrate
			err = db.Migrate()
			require.NoError(t, err)

			// verify results
			sqlDB, err := drv.Open()
			require.NoError(t, err)
			defer dbutil.MustClose(sqlDB)

			count := 0
			err = sqlDB.QueryRow(`select count(*) from schema_migrations
				where version = '20151129054053'`).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)
		})
	}
}

func TestUp(t *testing.T) {
	for _, u := range testURLs() {
		t.Run(u.Scheme, func(t *testing.T) {
			db := newTestDB(t, u)
			drv, err := db.GetDriver()
			require.NoError(t, err)

			// drop database
			err = db.Drop()
			require.NoError(t, err)

			// create and migrate
			err = db.CreateAndMigrate()
			require.NoError(t, err)

			// verify results
			sqlDB, err := drv.Open()
			require.NoError(t, err)
			defer dbutil.MustClose(sqlDB)

			count := 0
			err = sqlDB.QueryRow(`select count(*) from schema_migrations
				where version = '20151129054053'`).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)
		})
	}
}

func TestRollback(t *testing.T) {
	for _, u := range testURLs() {
		t.Run(u.Scheme, func(t *testing.T) {
			db := newTestDB(t, u)
			drv, err := db.GetDriver()
			require.NoError(t, err)

			// drop, recreate, and migrate database
			err = db.Drop()
			require.NoError(t, err)
			err = db.Create()
			require.NoError(t, err)
			err = db.Migrate()
			require.NoError(t, err)

			// verify migration
			sqlDB, err := drv.Open()
			require.NoError(t, err)
			defer dbutil.MustClose(sqlDB)

			count := 0
			err = sqlDB.QueryRow(`select count(*) from schema_migrations
				where version = '20151129054053'`).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
			require.Nil(t, err)

			// rollback
			err = db.Rollback()
			require.NoError(t, err)

			// verify rollback
			err = sqlDB.QueryRow("select count(*) from schema_migrations").Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
			require.NotNil(t, err)
			require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())
		})
	}
}

func TestStatus(t *testing.T) {
	for _, u := range testURLs() {
		t.Run(u.Scheme, func(t *testing.T) {
			db := newTestDB(t, u)
			drv, err := db.GetDriver()
			require.NoError(t, err)

			// drop, recreate, and migrate database
			err = db.Drop()
			require.NoError(t, err)
			err = db.Create()
			require.NoError(t, err)

			// verify migration
			sqlDB, err := drv.Open()
			require.NoError(t, err)
			defer dbutil.MustClose(sqlDB)

			// two pending
			results, err := db.CheckMigrationsStatus(drv)
			require.NoError(t, err)
			require.Len(t, results, 2)
			require.False(t, results[0].Applied)
			require.False(t, results[1].Applied)
			migrationsTableExists, err := drv.MigrationsTableExists(sqlDB)
			require.NoError(t, err)
			require.False(t, migrationsTableExists)

			// run migrations
			err = db.Migrate()
			require.NoError(t, err)

			// two applied
			results, err = db.CheckMigrationsStatus(drv)
			require.NoError(t, err)
			require.Len(t, results, 2)
			require.True(t, results[0].Applied)
			require.True(t, results[1].Applied)

			// rollback last migration
			err = db.Rollback()
			require.NoError(t, err)

			// one applied, one pending
			results, err = db.CheckMigrationsStatus(drv)
			require.NoError(t, err)
			require.Len(t, results, 2)
			require.True(t, results[0].Applied)
			require.False(t, results[1].Applied)
		})
	}
}
