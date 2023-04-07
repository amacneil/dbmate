package clickhouse

import (
	"database/sql"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

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
	drv := testClickHouseDriverOnCluster(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("clickhouse", drv.databaseURL.String())
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
		db, err := sql.Open("clickhouse", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.EqualError(t, err, "code: 81, message: Database dbmate_test doesn't exist")
	}()
}
