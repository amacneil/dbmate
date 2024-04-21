package clickhouse

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"

	"github.com/stretchr/testify/require"
)

func testClickHouseDriverURL(t *testing.T, u *url.URL) *Driver {
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func testClickHouseDriver(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "CLICKHOUSE_TEST_URL")
	return testClickHouseDriverURL(t, u)
}

func testClickHouseDriverCluster01(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "CLICKHOUSE_CLUSTER_01_TEST_URL")
	u.RawQuery = "on_cluster"
	return testClickHouseDriverURL(t, u)
}

func testClickHouseDriverCluster02(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "CLICKHOUSE_CLUSTER_02_TEST_URL")
	u.RawQuery = "on_cluster"
	return testClickHouseDriverURL(t, u)
}

func prepTestClickHouseDB(t *testing.T, drv *Driver) *sql.DB {
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
