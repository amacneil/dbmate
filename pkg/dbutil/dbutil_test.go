package dbutil_test

import (
	"database/sql"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	_ "github.com/mattn/go-sqlite3" // database/sql driver
	"github.com/stretchr/testify/require"
)

func TestDatabaseName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		u := dbutil.MustParseURL("foo://host/dbname?query")
		name := dbutil.DatabaseName(u)
		require.Equal(t, "dbname", name)
	})

	t.Run("empty", func(t *testing.T) {
		u := dbutil.MustParseURL("foo://host")
		name := dbutil.DatabaseName(u)
		require.Equal(t, "", name)
	})
}

func TestTrimLeadingSQLComments(t *testing.T) {
	in := "--\n" +
		"-- foo\n\n" +
		"-- bar\n\n" +
		"real stuff\n" +
		"-- end\n"
	out, err := dbutil.TrimLeadingSQLComments([]byte(in))
	require.NoError(t, err)
	require.Equal(t, "real stuff\n-- end\n", string(out))
}

// connect to in-memory sqlite database for testing
const sqliteMemoryDB = "file:dbutil.sqlite3?mode=memory&cache=shared"

func TestQueryColumn(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteMemoryDB)
	require.NoError(t, err)

	val, err := dbutil.QueryColumn(db, "select 'foo_' || val from (select ? as val union select ?)",
		"hi", "there")
	require.NoError(t, err)
	require.Equal(t, []string{"foo_hi", "foo_there"}, val)
}

func TestQueryValue(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteMemoryDB)
	require.NoError(t, err)

	val, err := dbutil.QueryValue(db, "select $1 + $2", "5", 2)
	require.NoError(t, err)
	require.Equal(t, "7", val)
}

func TestDSNDriverRequired(t *testing.T) {
	driver := "postgres"
	connStr := "user=User dbname=DBName"
	_, err := dbutil.NewDSN(driver, connStr)
	require.NoError(t, err)
	_, err = dbutil.NewDSN("", connStr)
	require.Error(t, err)
	_, err = dbutil.NewDSN("", connStr+" driver=postgres")
	require.NoError(t, err)
}

func TestDSNGetKey(t *testing.T) {
	driver := "postgres"
	connStr := "user=User dbname=DBName"
	dsn, err := dbutil.NewDSN(driver, connStr)
	require.NoError(t, err)
	require.Equal(t, connStr, dsn.ConnectionString())

	userValue := dsn.GetKey("user")
	require.Equal(t, "User", userValue)
	dbnameValue := dsn.GetKey("dbname")
	require.Equal(t, "DBName", dbnameValue)
}

func TestDSNDeleteKey(t *testing.T) {
	driver := "postgres"
	connStr := "user=User dbname=DBName"
	dsn, err := dbutil.NewDSN(driver, connStr)
	require.NoError(t, err)
	require.Equal(t, connStr, dsn.ConnectionString())

	userValue := dsn.DeleteKey("user")
	require.Equal(t, "User", userValue)
	require.Equal(t, "dbname=DBName", dsn.ConnectionString())
	dbnameValue := dsn.DeleteKey("dbname")
	require.Equal(t, "DBName", dbnameValue)
	require.Equal(t, "", dsn.ConnectionString())
}
