package bigquery

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"cloud.google.com/go/bigquery"
	"github.com/stretchr/testify/require"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

func testBigQueryDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(os.Getenv("BIGQUERY_TEST_URL"))
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestBigQueryDB(t *testing.T) *sql.DB {
	drv := testBigQueryDriver(t)

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
	db := dbmate.New(dbutil.MustParseURL("bigquery://"))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestGetClient(t *testing.T) {
	drv := testBigQueryDriver(t)

	db, err := drv.Open()
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	err = conn.Raw(func(driverConn any) error {
		value := reflect.ValueOf(driverConn).Elem().FieldByName("client")
		value = reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()

		client := value.Interface().(*bigquery.Client)
		require.Equal(t, "test", client.Project())

		connValue := reflect.ValueOf(driverConn).Elem()
		configField := connValue.FieldByName("config")

		projectField := configField.FieldByName("projectID")
		require.True(t, projectField.IsValid())
		require.Equal(t, "test", projectField.String())

		datasetField := configField.FieldByName("dataSet")
		require.True(t, datasetField.IsValid())
		require.Equal(t, "dbmate_test", datasetField.String())

		locationField := configField.FieldByName("location")
		require.True(t, locationField.IsValid())
		require.Equal(t, "asia-southeast1", locationField.String())

		return nil
	})
	require.NoError(t, err)
}

func TestConnectionString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"bigquery://projectid/dataset", "bigquery://projectid/dataset"},
		{"bigquery://projectid/location/dataset", "bigquery://projectid/location/dataset"},
		{"bigquery://projectid/location/dataset?disable_auth=false", "bigquery://projectid/location/dataset"},
		{"bigquery://projectid/location/dataset?disable_auth=true", "bigquery://projectid/location/dataset"},
		{"bigquery://projectid/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050", "bigquery://projectid/dataset"},
		{"bigquery://projectid/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&disable_auth=true", "bigquery://projectid/location/dataset"},
		{"bigquery://bigquery:9050/test/dbmate_test?disable_auth=true", "bigquery://test/dbmate_test?disable_auth=true&endpoint=http%3A%2F%2Fbigquery%3A9050"},
		{"bigquery://0.0.0.0:9050/project/location/dataset?disable_auth=true", "bigquery://project/location/dataset?disable_auth=true&endpoint=http%3A%2F%2F0.0.0.0%3A9050"},
		{"bigquery://bigquery:9050/test/dbmate_test?disable_auth=false", "bigquery://test/dbmate_test?endpoint=http%3A%2F%2Fbigquery%3A9050"},
		{"bigquery://0.0.0.0:9050/project/location/dataset", "bigquery://project/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := connectionString(u)
			require.Equal(t, c.expected, actual)
		})
	}
}
func TestBigQueryCreateDropDatabase(t *testing.T) {
	drv := testBigQueryDriver(t)

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

func TestBigQueryDatabaseExists(t *testing.T) {
	drv := testBigQueryDriver(t)

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

func TestBigQueryCreateMigrationsTable(t *testing.T) {
	drv := testBigQueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestBigQueryDB(t)
	defer dbutil.MustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.Error(t, err)
	require.Regexp(t, "Table not found: test_migrations", err.Error())

	// create table
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)
}

func TestBigQuerySelectMigrations(t *testing.T) {
	drv := testBigQueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestBigQueryDB(t)
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

func TestBigQueryInsertMigration(t *testing.T) {
	drv := testBigQueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestBigQueryDB(t)
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

func TestBigQueryDeleteMigration(t *testing.T) {
	drv := testBigQueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestBigQueryDB(t)
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

func TestBigQueryPingError(t *testing.T) {
	drv := testBigQueryDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.Error(t, err)
	require.Contains(t, err.Error(), "dataset dbmate_test is not found")
}

func TestBigQueryPingSuccess(t *testing.T) {
	drv := testBigQueryDriver(t)

	db := prepTestBigQueryDB(t)
	defer dbutil.MustClose(db)

	// ping database
	err := drv.Ping()
	require.NoError(t, err)
}

func TestBigQueryMigrationsTableExists(t *testing.T) {
	drv := testBigQueryDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestBigQueryDB(t)
	defer dbutil.MustClose(db)

	exists, err := drv.MigrationsTableExists(db)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	exists, err = drv.MigrationsTableExists(db)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}
