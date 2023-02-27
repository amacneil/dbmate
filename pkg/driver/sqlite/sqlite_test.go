//go:build cgo
// +build cgo

package sqlite

import (
	"database/sql"
	"os"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testSQLiteDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(os.Getenv("SQLITE_TEST_URL"))
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
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

func TestGetDriver(t *testing.T) {
	db := dbmate.New(dbutil.MustParseURL("sqlite://"))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	t.Run("relative", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:foo/bar.sqlite3?mode=ro")
		require.Equal(t, "foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with dot", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:./foo/bar.sqlite3?mode=ro")
		require.Equal(t, "./foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with double dot", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:../foo/bar.sqlite3?mode=ro")
		require.Equal(t, "../foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("absolute", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:/tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("two slashes", func(t *testing.T) {
		// interpreted as absolute path
		u := dbutil.MustParseURL("sqlite://tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes", func(t *testing.T) {
		// interpreted as absolute path
		u := dbutil.MustParseURL("sqlite:///tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("four slashes", func(t *testing.T) {
		// interpreted as absolute path
		// supported for backwards compatibility
		u := dbutil.MustParseURL("sqlite:////tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:foo bar.sqlite3?mode=ro")
		require.Equal(t, "foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space and dot", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:./foo bar.sqlite3?mode=ro")
		require.Equal(t, "./foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space and double dot", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:../foo bar.sqlite3?mode=ro")
		require.Equal(t, "../foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("absolute with space", func(t *testing.T) {
		u := dbutil.MustParseURL("sqlite:/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("two slashes with space in path", func(t *testing.T) {
		// interpreted as absolute path
		u := dbutil.MustParseURL("sqlite://tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes with space in path", func(t *testing.T) {
		// interpreted as absolute path
		u := dbutil.MustParseURL("sqlite:///tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes with space in path (1st dir)", func(t *testing.T) {
		// interpreted as absolute path
		u := dbutil.MustParseURL("sqlite:///tm p/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tm p/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("four slashes with space", func(t *testing.T) {
		// interpreted as absolute path
		// supported for backwards compatibility
		u := dbutil.MustParseURL("sqlite:////tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})
}

func TestSQLiteCreateDropDatabase(t *testing.T) {
	drv := testSQLiteDriver(t)
	path := ConnectionString(drv.databaseURL)

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
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.NoError(t, err)

	// create a table that will trigger `sqlite_sequence` system table
	_, err = db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)")
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)")
	require.Contains(t, string(schema), "CREATE TABLE IF NOT EXISTS \"test_migrations\"")
	require.Contains(t, string(schema), ");\n-- Dbmate schema migrations\n"+
		"INSERT INTO \"test_migrations\" (version) VALUES\n"+
		"  ('abc1'),\n"+
		"  ('abc2');\n")

	// sqlite_* tables should not be present in the dump (.schema --nosys)
	require.NotContains(t, string(schema), "sqlite_")

	// DumpSchema should return error if command fails
	drv.databaseURL = dbutil.MustParseURL(".")
	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.Error(t, err)
	require.EqualError(t, err, "Error: unable to open database \"/.\": unable to open database file")
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
		defer dbutil.MustClose(db)

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
		defer dbutil.MustClose(db)

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
	defer dbutil.MustClose(db)

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
	defer dbutil.MustClose(db)

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
	defer dbutil.MustClose(db)

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
	path := ConnectionString(drv.databaseURL)

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
