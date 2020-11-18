package dbutil_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/amacneil/dbmate/pkg/dbutil"
	"github.com/amacneil/dbmate/pkg/driver/sqlite"

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

func TestQueryColumn(t *testing.T) {
	u := dbutil.MustParseURL(os.Getenv("SQLITE_TEST_URL"))
	db, err := sql.Open("sqlite3", sqlite.ConnectionString(u))
	require.NoError(t, err)

	val, err := dbutil.QueryColumn(db, "select 'foo_' || atom from json_each(?)", `["hi", "there"]`)
	require.NoError(t, err)
	require.Equal(t, []string{"foo_hi", "foo_there"}, val)
}

func TestQueryValue(t *testing.T) {
	u := dbutil.MustParseURL(os.Getenv("SQLITE_TEST_URL"))
	db, err := sql.Open("sqlite3", sqlite.ConnectionString(u))
	require.NoError(t, err)

	val, err := dbutil.QueryValue(db, "select $1 + $2", "5", 2)
	require.NoError(t, err)
	require.Equal(t, "7", val)
}
