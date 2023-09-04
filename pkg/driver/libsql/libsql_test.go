//go:build cgo
// +build cgo

package libsql

import (
	"database/sql"
	"os"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testLibSQLDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(os.Getenv("LIBSQL_TEST_URL"))
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestLibSQLDB(t *testing.T) *sql.DB {
	drv := testLibSQLDriver(t)

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
	db := dbmate.New(dbutil.MustParseURL("libsql://"))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	t.Run("libsql protocol", func(t *testing.T) {
		u := dbutil.MustParseURL("libsql://example.com/db")
		require.Equal(t, "libsql://example.com/db", ConnectionString(u))
	})

	t.Run("http protocol", func(t *testing.T) {
		u := dbutil.MustParseURL("http://example.com/db")
		require.Equal(t, "http://example.com/db", ConnectionString(u))
	})

	t.Run("https protocol", func(t *testing.T) {
		u := dbutil.MustParseURL("https://example.com/db")
		require.Equal(t, "https://example.com/db", ConnectionString(u))
	})
}

func TestLibSQLDumpSchema(t *testing.T) {
	drv := testLibSQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestLibSQLDB(t)
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
	require.Contains(t, string(schema), "CREATE TABLE \"test_migrations\"")
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
	require.EqualError(t, err, "Error: unable to open database file: is a directory")
}

func TestLibSQLDatabaseExists(t *testing.T) {
	drv := testLibSQLDriver(t)

	// create database
	err := drv.CreateDatabase()
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err := drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestLibSQLCreateMigrationsTable(t *testing.T) {
	t.Run("default table", func(t *testing.T) {
		drv := testLibSQLDriver(t)
		db := prepTestLibSQLDB(t)
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
		drv := testLibSQLDriver(t)
		drv.migrationsTableName = "test_migrations"

		db := prepTestLibSQLDB(t)
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

func TestLibSQLSelectMigrations(t *testing.T) {
	drv := testLibSQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestLibSQLDB(t)
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

func TestLibSQLInsertMigration(t *testing.T) {
	drv := testLibSQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestLibSQLDB(t)
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

func TestLibSQLDeleteMigration(t *testing.T) {
	drv := testLibSQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestLibSQLDB(t)
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

func TestLibSQLPing(t *testing.T) {
	drv := testLibSQLDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.NoError(t, err)
}

func TestLibSQLQuotedMigrationsTableName(t *testing.T) {
	t.Run("default name", func(t *testing.T) {
		drv := testLibSQLDriver(t)
		name := drv.quotedMigrationsTableName()
		require.Equal(t, `"schema_migrations"`, name)
	})

	t.Run("custom name", func(t *testing.T) {
		drv := testLibSQLDriver(t)
		drv.migrationsTableName = "fooMigrations"

		name := drv.quotedMigrationsTableName()
		require.Equal(t, `"fooMigrations"`, name)
	})
}
