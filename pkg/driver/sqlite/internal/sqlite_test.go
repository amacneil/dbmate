package internal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/amacneil/dbmate/v2/pkg/dbtest"
)

func TestConnectionString(t *testing.T) {
	t.Run("relative", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:foo/bar.sqlite3?mode=ro")
		require.Equal(t, "foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with dot", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:./foo/bar.sqlite3?mode=ro")
		require.Equal(t, "./foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with double dot", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:../foo/bar.sqlite3?mode=ro")
		require.Equal(t, "../foo/bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("absolute", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:/tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("two slashes", func(t *testing.T) {
		// interpreted as absolute path
		u := dbtest.MustParseURL(t, "sqlite://tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes", func(t *testing.T) {
		// interpreted as absolute path
		u := dbtest.MustParseURL(t, "sqlite:///tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("four slashes", func(t *testing.T) {
		// interpreted as absolute path
		// supported for backwards compatibility
		u := dbtest.MustParseURL(t, "sqlite:////tmp/foo.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:foo bar.sqlite3?mode=ro")
		require.Equal(t, "foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space and dot", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:./foo bar.sqlite3?mode=ro")
		require.Equal(t, "./foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("relative with space and double dot", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:../foo bar.sqlite3?mode=ro")
		require.Equal(t, "../foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("absolute with space", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "sqlite:/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("two slashes with space in path", func(t *testing.T) {
		// interpreted as absolute path
		u := dbtest.MustParseURL(t, "sqlite://tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes with space in path", func(t *testing.T) {
		// interpreted as absolute path
		u := dbtest.MustParseURL(t, "sqlite:///tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("three slashes with space in path (1st dir)", func(t *testing.T) {
		// interpreted as absolute path
		u := dbtest.MustParseURL(t, "sqlite:///tm p/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tm p/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})

	t.Run("four slashes with space", func(t *testing.T) {
		// interpreted as absolute path
		// supported for backwards compatibility
		u := dbtest.MustParseURL(t, "sqlite:////tmp/foo bar.sqlite3?mode=ro")
		require.Equal(t, "/tmp/foo bar.sqlite3?mode=ro", ConnectionString(u))
	})
}
