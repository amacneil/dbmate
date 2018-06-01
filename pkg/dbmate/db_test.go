package dbmate

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testdataDir string

func newTestDB(t *testing.T, u *url.URL) *DB {
	var err error

	// only chdir once, because testdata is relative to current directory
	if testdataDir == "" {
		testdataDir, err = filepath.Abs("../../testdata")
		require.NoError(t, err)

		err = os.Chdir(testdataDir)
		require.NoError(t, err)
	}

	db := New(u)
	db.AutoDumpSchema = false

	return db
}

func TestNew(t *testing.T) {
	u := postgresTestURL(t)
	db := New(u)
	require.True(t, db.AutoDumpSchema)
	require.Equal(t, u.String(), db.DatabaseURL.String())
	require.Equal(t, "./db/migrations", db.MigrationsDir)
	require.Equal(t, "./db/schema.sql", db.SchemaFile)
	require.Equal(t, time.Second, db.WaitInterval)
	require.Equal(t, 60*time.Second, db.WaitTimeout)
}

func TestWait(t *testing.T) {
	u := postgresTestURL(t)
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
	u := postgresTestURL(t)
	db := newTestDB(t, u)

	// create custom schema file directory
	dir, err := ioutil.TempDir("", "dbmate")
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
	err = db.CreateAndMigrate(false)
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// dump schema
	err = db.DumpSchema()
	require.NoError(t, err)

	// verify schema
	schema, err := ioutil.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")
}

func TestAutoDumpSchema(t *testing.T) {
	u := postgresTestURL(t)
	db := newTestDB(t, u)
	db.AutoDumpSchema = true

	// create custom schema file directory
	dir, err := ioutil.TempDir("", "dbmate")
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
	err = db.CreateAndMigrate(false)
	require.NoError(t, err)

	// verify schema
	schema, err := ioutil.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")

	// remove schema
	err = os.Remove(db.SchemaFile)
	require.NoError(t, err)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)

	// schema should be recreated
	schema, err = ioutil.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- PostgreSQL database dump")
}

func testURLs(t *testing.T) []*url.URL {
	return []*url.URL{
		postgresTestURL(t),
		mySQLTestURL(t),
		sqliteTestURL(t),
	}
}

func testMigrateURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop and recreate database
	err := db.Drop()
	require.NoError(t, err)
	err = db.Create(false)
	require.NoError(t, err)

	// migrate
	err = db.Migrate(false)
	require.NoError(t, err)

	// verify results
	sqlDB, err := GetDriverOpen(u)
	require.NoError(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMigrate(t *testing.T) {
	for _, u := range testURLs(t) {
		testMigrateURL(t, u)
	}
}

func testUpURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop database
	err := db.Drop()
	require.NoError(t, err)

	// create and migrate
	err = db.CreateAndMigrate(false)
	require.NoError(t, err)

	// verify results
	sqlDB, err := GetDriverOpen(u)
	require.NoError(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestUp(t *testing.T) {
	for _, u := range testURLs(t) {
		testUpURL(t, u)
	}
}

func testRollbackURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop, recreate, and migrate database
	err := db.Drop()
	require.NoError(t, err)
	err = db.Create(false)
	require.NoError(t, err)
	err = db.Migrate(false)
	require.NoError(t, err)

	// verify migration
	sqlDB, err := GetDriverOpen(u)
	require.NoError(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)

	// verify rollback
	err = sqlDB.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.NotNil(t, err)
	require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())
}

func TestRollback(t *testing.T) {
	for _, u := range testURLs(t) {
		testRollbackURL(t, u)
	}
}
