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

func prepTestSQLiteDB(t *testing.T) *sql.DB {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// connect database
	db, err := drv.Open(u)
	require.NoError(t, err)

	return db
}

func TestSQLiteCreateDropDatabase(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)
	path := sqlitePath(u)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// check that database exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// drop the database
	err = drv.DropDatabase(u)
	require.NoError(t, err)

	// check that database no longer exists
	_, err = os.Stat(path)
	require.NotNil(t, err)
	require.Equal(t, true, os.IsNotExist(err))
}

func TestSQLiteDumpSchema(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

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
	schema, err := drv.DumpSchema(u, db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE schema_migrations")
	require.Contains(t, string(schema), ");\n-- Dbmate schema migrations\n"+
		"INSERT INTO schema_migrations (version) VALUES\n"+
		"  ('abc1'),\n"+
		"  ('abc2');\n")

	// DumpSchema should return error if command fails
	u.Path = "/."
	schema, err = drv.DumpSchema(u, db)
	require.Nil(t, schema)
	require.EqualError(t, err, "Error: unable to open database \".\": "+
		"unable to open database file")
}

func TestSQLiteDatabaseExists(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestSQLiteCreateMigrationsTable(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
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
}

func TestSQLiteSelectMigrations(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
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
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").
		Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSQLiteDeleteMigration(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc1'), ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSQLitePing(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)
	path := sqlitePath(u)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// ping database
	err = drv.Ping(u)
	require.NoError(t, err)

	// check that the database was created (sqlite-only behavior)
	_, err = os.Stat(path)
	require.NoError(t, err)

	// drop the database
	err = drv.DropDatabase(u)
	require.NoError(t, err)

	// create directory where database file is expected
	err = os.Mkdir(path, 0755)
	require.NoError(t, err)
	defer func() {
		err = os.RemoveAll(path)
		require.NoError(t, err)
	}()

	// ping database should fail
	err = drv.Ping(u)
	require.EqualError(t, err, "unable to open database file: is a directory")
}
