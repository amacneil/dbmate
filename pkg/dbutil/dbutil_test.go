package dbutil_test

import (
	"database/sql"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	_ "github.com/mattn/go-sqlite3" // database/sql driver
	"github.com/stretchr/testify/require"
)

func TestDatabaseName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "foo://host/dbname?query")
		name := dbutil.DatabaseName(u)
		require.Equal(t, "dbname", name)
	})

	t.Run("empty", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "foo://host")
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

func TestStripPsqlMetaCommands(t *testing.T) {
	t.Run("strips restrict and unrestrict", func(t *testing.T) {
		in := "\\restrict dbmate\n" +
			"-- comment\n" +
			"SET statement_timeout = 0;\n" +
			"CREATE TABLE users (id int);\n" +
			"\\unrestrict dbmate\n"
		out, err := dbutil.StripPsqlMetaCommands([]byte(in))
		require.NoError(t, err)
		expected := "-- comment\n" +
			"SET statement_timeout = 0;\n" +
			"CREATE TABLE users (id int);\n"
		require.Equal(t, expected, string(out))
	})

	t.Run("strips indented backslash commands", func(t *testing.T) {
		in := "  \\restrict dbmate\n" +
			"SELECT 1;\n" +
			"\t\\unrestrict dbmate\n"
		out, err := dbutil.StripPsqlMetaCommands([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "SELECT 1;\n", string(out))
	})

	t.Run("preserves non-backslash content", func(t *testing.T) {
		in := "-- This is a comment\n" +
			"CREATE TABLE test (name varchar(100));\n" +
			"INSERT INTO test VALUES ('hello\\world');\n"
		out, err := dbutil.StripPsqlMetaCommands([]byte(in))
		require.NoError(t, err)
		require.Equal(t, in, string(out))
	})

	t.Run("handles empty input", func(t *testing.T) {
		out, err := dbutil.StripPsqlMetaCommands([]byte(""))
		require.NoError(t, err)
		require.Equal(t, "", string(out))
	})

	t.Run("strips all backslash commands", func(t *testing.T) {
		// Any line starting with backslash is a psql meta-command
		in := "\\connect dbname\n" +
			"SELECT 1;\n" +
			"\\quit\n"
		out, err := dbutil.StripPsqlMetaCommands([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "SELECT 1;\n", string(out))
	})
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
