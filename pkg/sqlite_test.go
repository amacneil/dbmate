package pkg

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func sqliteTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("sqlite3:////tmp/dbmate.sqlite3")
	require.Nil(t, err)

	return u
}

func prepTestSQLiteDB(t *testing.T) *sql.DB {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// connect database
	db, err := drv.Open(u)
	require.Nil(t, err)

	return db
}

func TestSQLiteCreateDropDatabase(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// check that database exists
	_, err = os.Stat(sqlitePath(u))
	require.Nil(t, err)

	// drop the database
	err = drv.DropDatabase(u)
	require.Nil(t, err)

	// check that database no longer exists
	_, err = os.Stat(sqlitePath(u))
	require.NotNil(t, err)
	require.Equal(t, true, os.IsNotExist(err))
}

func TestSQLiteDatabaseExists(t *testing.T) {
	drv := SQLiteDriver{}
	u := sqliteTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.Nil(t, err)
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
	require.Nil(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)
}

func TestSQLiteSelectMigrations(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.Nil(t, err)

	migrations, err := drv.SelectMigrations(db, -1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
}

func TestSQLiteInsertMigration(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.Nil(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").
		Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestSQLiteDeleteMigration(t *testing.T) {
	drv := SQLiteDriver{}
	db := prepTestSQLiteDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc1'), ('abc2')`)
	require.Nil(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}
