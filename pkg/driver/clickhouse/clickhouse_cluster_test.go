package clickhouse

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testClickHouseDriverCluster01(t *testing.T) *Driver {
	u := fmt.Sprintf("%s?on_cluster", os.Getenv("CLICKHOUSE_CLUSTER_01_TEST_URL"))
	return testClickHouseDriverURL(t, u)
}

func testClickHouseDriverCluster02(t *testing.T) *Driver {
	u := fmt.Sprintf("%s?on_cluster", os.Getenv("CLICKHOUSE_CLUSTER_02_TEST_URL"))
	return testClickHouseDriverURL(t, u)
}

func assertDatabaseExists(t *testing.T, drv *Driver, shouldExist bool){
	db, err := sql.Open("clickhouse", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		if shouldExist{
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, "code: 81, message: Database dbmate_test doesn't exist")
		}
}

func TestOnClusterClause(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// not on cluster
		{"clickhouse://myhost:9000", ""},
		// on_cluster supplied
		{"clickhouse://myhost:9000?on_cluster", " ON CLUSTER '{cluster}'"},
		// on_cluster with supplied macro
		{"clickhouse://myhost:9000?on_cluster&cluster_macro={cluster2}", " ON CLUSTER '{cluster2}'"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			drv := testClickHouseDriverURL(t, c.input)

			actual := drv.onClusterClause()
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestClickHouseCreateDropDatabaseOnCluster(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)

	// drop any existing database
	err := drv01.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv01.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	assertDatabaseExists(t, drv01, true)
	// check that database exists on the other clickhouse node and we can connect to it
	assertDatabaseExists(t, drv02, true)

	// drop the database
	err = drv01.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	assertDatabaseExists(t, drv01, false)
	// check that database no longer exists on the other clickhouse node
	assertDatabaseExists(t, drv02, false)
}

func TestClickHouseDumpSchemaOnCluster(t *testing.T) {
	drv := testClickHouseDriverCluster01(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestClickHouseDB(t, drv)
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	tx, err := db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)
	tx, err = db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc2")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE "+drv.databaseName()+".test_migrations")
	require.Contains(t, string(schema), "--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"INSERT INTO test_migrations (version) VALUES\n"+
		"    ('abc1'),\n"+
		"    ('abc2');\n")

	// DumpSchema should return error if command fails
	drv.databaseURL.Path = "/fakedb"
	db, err = sql.Open("clickhouse", drv.databaseURL.String())
	require.NoError(t, err)

	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.EqualError(t, err, "code: 81, message: Database fakedb doesn't exist")
}
