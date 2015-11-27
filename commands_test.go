package main_test

import (
	"flag"
	"github.com/adrianmacneil/dbmate"
	"github.com/codegangsta/cli"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func newContext() *cli.Context {
	app := main.NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		f.Apply(flagset)
	}

	return cli.NewContext(app, flagset, nil)
}

func TestGetDatabaseUrl_Default(t *testing.T) {
	err := os.Setenv("DATABASE_URL", "postgres://example.org/db")
	require.Nil(t, err)

	ctx := newContext()
	u, err := main.GetDatabaseURL(ctx)
	require.Nil(t, err)

	require.Equal(t, "postgres", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}
