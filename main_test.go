package main

import (
	"flag"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func testContext(t *testing.T, u *url.URL) *cli.Context {
	err := os.Setenv("DATABASE_URL", u.String())
	require.NoError(t, err)

	app := NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		f.Apply(flagset)
	}

	return cli.NewContext(app, flagset, nil)
}

func TestGetDatabaseUrl(t *testing.T) {
	envURL, err := url.Parse("foo://example.org/db")
	require.NoError(t, err)
	ctx := testContext(t, envURL)

	u, err := getDatabaseURL(ctx)
	require.NoError(t, err)

	require.Equal(t, "foo", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}

func TestRedactLogString(t *testing.T) {
	examples := []struct {
		in       string
		expected string
	}{
		{"normal string",
			"normal string"},
		// malformed URL example (note forward slash in password)
		{"parse \"mysql://username:otS33+tb/e4=@localhost:3306/database\": invalid",
			"parse \"mysql://username:********@localhost:3306/database\": invalid"},
		// invalid port, but probably not a password since there is no @
		{"parse \"mysql://localhost:abc/database\": invalid",
			"parse \"mysql://localhost:abc/database\": invalid"},
	}

	for _, ex := range examples {
		require.Equal(t, ex.expected, redactLogString(ex.in))
	}
}
