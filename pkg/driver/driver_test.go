package driver_test

import (
	"testing"

	"github.com/flowhamster/dbmate/pkg/driver"
	"github.com/flowhamster/dbmate/pkg/driver/mysql"
	"github.com/flowhamster/dbmate/pkg/driver/postgres"
	_ "github.com/flowhamster/dbmate/pkg/driver/sqlite"

	"github.com/stretchr/testify/require"
)

func TestGetDriver_Postgres(t *testing.T) {
	drv, err := driver.GetDriver("postgres")
	require.Nil(t, err)
	_, ok := drv.(postgres.PostgresDriver)
	require.Equal(t, true, ok)
}

func TestGetDriver_MySQL(t *testing.T) {
	drv, err := driver.GetDriver("mysql")
	require.Nil(t, err)
	_, ok := drv.(mysql.MySQLDriver)
	require.Equal(t, true, ok)
}

func TestGetDriver_Error(t *testing.T) {
	drv, err := driver.GetDriver("foo")
	require.Equal(t, "Unknown driver: foo", err.Error())
	require.Nil(t, drv)
}
