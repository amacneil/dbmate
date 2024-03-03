package bigquery

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

func getURL() string {
	return os.Getenv("BIGQUERY_TEST_URL")
}

func getDbConnection(url string) (*sql.DB, error) {
	return sql.Open("bigquery", url)
}

func testBigqueryDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(getURL())
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestDB(t *testing.T) *sql.DB {
	drv := testBigqueryDriver(t)

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
	db := dbmate.New(dbutil.MustParseURL(getURL()))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	cases := [4]string{
		"bigquery://projectid/dataset",
		"bigquery://projectid/location/dataset",
		"bigquery://projectid/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050",
		"bigquery://projectid/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&disable_auth=true",
	}

	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := url.Parse(c)
			require.NoError(t, err)
		})
	}
}

func TestMySQLCreateDropDatabase(t *testing.T) {
	drv := testBigqueryDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.NoError(t, err)
	}()

	// drop the database
	err = drv.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	func() {
		db, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.Error(t, err)
		require.Regexp(t, "dataset dbmate_test is not found", err.Error())
	}()
}

func TestBigqueryCreateAndInsertMigration(t *testing.T) {
	drv := testBigqueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestDB(t)
	defer dbutil.MustClose(db)

	// create migrations table
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.NoError(t, err)
}

func TestMySQLCreateMigrationsTable(t *testing.T) {
	drv := testBigqueryDriver(t)
	drv.migrationsTableName = "test_migrations_1"

	db, err := getDbConnection(drv.databaseURL.String())
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	// migrations table should not exist
	count := 0
	err = db.QueryRow("select count(*) from test_migrations_1").Scan(&count)
	require.Error(t, err)

	// create table
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from test_migrations_1").Scan(&count)
	require.NoError(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)
}

func TestMySQLSelectMigrations(t *testing.T) {
	drv := testBigqueryDriver(t)
	drv.migrationsTableName = "test_migrations_2"

	db, err := getDbConnection(drv.databaseURL.String())
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations_2 (version)
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

func TestMySQLInsertMigration(t *testing.T) {
	drv := testBigqueryDriver(t)
	drv.migrationsTableName = "test_migrations_3"

	db, err := getDbConnection(drv.databaseURL.String())
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations_3").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from test_migrations_3 where version = 'abc1'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLDeleteMigration(t *testing.T) {
	drv := testBigqueryDriver(t)
	drv.migrationsTableName = "test_migrations_4"

	db, err := getDbConnection(drv.databaseURL.String())
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations_4 (version) values ('abc1'), ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations_4").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLDatabaseExists(t *testing.T) {
	drv := testBigqueryDriver(t)

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
