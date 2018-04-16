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
	require.Nil(t, err)

	app := NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		f.Apply(flagset)
	}

	return cli.NewContext(app, flagset, nil)
}

func TestGetDatabaseUrl(t *testing.T) {
	envURL, err := url.Parse("foo://example.org/db")
	require.Nil(t, err)
	ctx := testContext(t, envURL)

	u, err := getDatabaseURL(ctx)
	require.Nil(t, err)

	require.Equal(t, "foo", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}
