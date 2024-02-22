package clickhouse

import (
	"database/sql"
	"os"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testClickHouseDriverURL(t *testing.T, url string) *Driver {
	u := dbutil.MustParseURL(url)
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func testClickHouseDriver(t *testing.T) *Driver {
	return testClickHouseDriverURL(t, os.Getenv("CLICKHOUSE_TEST_URL"))
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
