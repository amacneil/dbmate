package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestDatabaseName(t *testing.T) {
	u, err := url.Parse("ignore://localhost/foo?query")
	require.NoError(t, err)

	name := databaseName(u)
	require.Equal(t, "foo", name)
}

func TestDatabaseName_Empty(t *testing.T) {
	u, err := url.Parse("ignore://localhost")
	require.NoError(t, err)

	name := databaseName(u)
	require.Equal(t, "", name)
}

func TestTrimLeadingSQLComments(t *testing.T) {
	in := "--\n" +
		"-- foo\n\n" +
		"-- bar\n\n" +
		"real stuff\n" +
		"-- end\n"
	out, err := trimLeadingSQLComments([]byte(in))
	require.NoError(t, err)
	require.Equal(t, "real stuff\n-- end\n", string(out))
}

func TestQueryColumn(t *testing.T) {
	u := postgresTestURL(t)
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)

	val, err := queryColumn(db, "select concat('foo_', unnest($1::text[]))",
		pq.Array([]string{"hi", "there"}))
	require.NoError(t, err)
	require.Equal(t, []string{"foo_hi", "foo_there"}, val)
}

func TestQueryValue(t *testing.T) {
	u := postgresTestURL(t)
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)

	val, err := queryValue(db, "select $1::int + $2::int", "5", 2)
	require.NoError(t, err)
	require.Equal(t, "7", val)
}
