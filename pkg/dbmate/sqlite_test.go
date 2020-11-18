// +build cgo

package dbmate

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func sqliteTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("sqlite3:////tmp/dbmate.sqlite3")
	require.NoError(t, err)

	return u
}

func testSQLiteDriver(t *testing.T) *SQLiteDriver {
	u := sqliteTestURL(t)
	drv, err := New(u).GetDriver()
	require.NoError(t, err)

	return drv.(*SQLiteDriver)
}

func prepTestSQLiteDB(t *testing.T) *sql.DB {
	drv := testSQLiteDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// connect database
	db, err := drv.Open()
	require.NoError(t, err)

	return db
}

func TestSQLiteCreateDropDatabase(t *testing.T) {
	drv := testSQLiteDriver(t)
	path := sqlitePath(drv.databaseURL)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// drop the database
	err = drv.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	_, err = os.Stat(path)
	require.NotNil(t, err)
	require.Equal(t, true, os.IsNotExist(err))
}

func TestSQLiteDumpSchema(t *testing.T) {
	drv := testSQLiteDriver(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestSQLiteDB(t)
	defer mustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE IF NOT EXISTS \"test_migrations\"")
	require.Contains(t, string(schema), ");\n-- Dbmate schema migrations\n"+
		"INSERT INTO \"test_migrations\" (version) VALUES\n"+
		"  ('abc1'),\n"+
		"  ('abc2');\n")

	// DumpSchema should return error if command fails
	drv.databaseURL.Path = "/."
	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.Error(t, err)
	require.EqualError(t, err, "Error: unable to open database \".\": "+
		"unable to open database file")
}

func TestSQLiteDatabaseExists(t *testing.T) {
	drv := testSQLiteDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestSQLiteCreateMigrationsTable(t *testing.T) {
	t.Run("default table", func(t *testing.T) {
		drv := testSQLiteDriver(t)
		db := prepTestSQLiteDB(t)
		defer mustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.Error(t, err)
		require.Regexp(t, "no such table: schema_migrations", err.Error())

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})

	t.Run("custom table", func(t *testing.T) {
		drv := testSQLiteDriver(t)
		drv.migrationsTableName = "test_migrations"

		db := prepTestSQLiteDB(t)
		defer mustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from test_migrations").Scan(&count)
		require.Error(t, err)
		require.Regexp(t, "no such table: test_migrations", err.Error())

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})
}

func TestSQLiteSelectMigrations(t *testing.T) {
	drv := testSQLiteDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.NoError(t, err)

	migrations, err := drv.SelectMigrations(db, -1)
	require.NoError(t, err)
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1)
	require.NoError(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
}

func TestSQLiteInsertMigration(t *testing.T) {
	drv := testSQLiteDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from test_migrations where version = 'abc1'").
		Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSQLiteDeleteMigration(t *testing.T) {
	drv := testSQLiteDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations (version)
		values ('abc1'), ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSQLitePing(t *testing.T) {
	drv := testSQLiteDriver(t)
	path := sqlitePath(drv.databaseURL)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.NoError(t, err)

	// check that the database was created (sqlite-only behavior)
	_, err = os.Stat(path)
	require.NoError(t, err)

	// drop the database
	err = drv.DropDatabase()
	require.NoError(t, err)

	// create directory where database file is expected
	err = os.Mkdir(path, 0755)
	require.NoError(t, err)
	defer func() {
		err = os.RemoveAll(path)
		require.NoError(t, err)
	}()

	// ping database should fail
	err = drv.Ping()
	require.EqualError(t, err, "unable to open database file: is a directory")
}

func TestSQLiteQuotedMigrationsTableName(t *testing.T) {
	t.Run("default name", func(t *testing.T) {
		drv := testSQLiteDriver(t)
		name := drv.quotedMigrationsTableName()
		require.Equal(t, `"schema_migrations"`, name)
	})

	t.Run("custom name", func(t *testing.T) {
		drv := testSQLiteDriver(t)
		drv.migrationsTableName = "fooMigrations"

		name := drv.quotedMigrationsTableName()
		require.Equal(t, `"fooMigrations"`, name)
	})
}
