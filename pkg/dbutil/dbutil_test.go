package dbutil_test

import (
	"bufio"
	"database/sql"
	"fmt"
	"strings"
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

func TestTrimLeadingSQLCommentsShortLineAtEOF(t *testing.T) {
	// >4KB of leading comments forces bufio.Scanner to slide its buffer,
	// leaving stale bytes just past the final token; a one-byte last line
	// must not be misread as a comment marker.
	var b strings.Builder
	for b.Len() < 4090 {
		b.WriteString("--\n")
	}
	for b.Len() < 4095 {
		b.WriteString("-")
	}
	b.WriteString("\n-") // final line: one byte, no trailing newline

	out, err := dbutil.TrimLeadingSQLComments([]byte(b.String()))
	require.NoError(t, err)
	require.Equal(t, "-\n", string(out))
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

func TestSQLFiltersLongLines(t *testing.T) {
	filters := []struct {
		name          string
		filter        func([]byte) ([]byte, error)
		skippedPrefix string
	}{
		{"TrimLeadingSQLComments", dbutil.TrimLeadingSQLComments, "-- "},
		{"StripPsqlMetaCommands", dbutil.StripPsqlMetaCommands, "\\restrict "},
	}
	for _, filter := range filters {
		t.Run(filter.name, func(t *testing.T) {
			for _, size := range []int{bufio.MaxScanTokenSize - 1, bufio.MaxScanTokenSize, bufio.MaxScanTokenSize + 1, 1024 * 1024} {
				for _, ending := range []struct {
					name  string
					value string
				}{
					{"LF", "\n"},
					{"CRLF", "\r\n"},
					{"EOF", ""},
				} {
					t.Run(fmt.Sprintf("SQL/%d/%s", size, ending.name), func(t *testing.T) {
						prefix, suffix := "CREATE TABLE test (value TEXT CHECK (value <> '", "'));"
						line := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
						out, err := filter.filter([]byte(line + ending.value))
						require.NoError(t, err)
						require.Equal(t, line+"\n", string(out))
					})
				}
			}

			t.Run("skipped long line", func(t *testing.T) {
				line := filter.skippedPrefix + strings.Repeat("x", 2*bufio.MaxScanTokenSize)
				out, err := filter.filter([]byte(line + "\nSELECT 1;\n"))
				require.NoError(t, err)
				require.Equal(t, "SELECT 1;\n", string(out))

				out, err = filter.filter([]byte(line))
				require.NoError(t, err)
				require.Empty(t, out)
			})
		})
	}
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
