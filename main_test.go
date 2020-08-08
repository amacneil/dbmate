package main

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestGetDatabaseUrl(t *testing.T) {
	// set environment variables
	require.NoError(t, os.Setenv("DATABASE_URL", "foo://example.org/one"))
	require.NoError(t, os.Setenv("CUSTOM_URL", "foo://example.org/two"))

	app := NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		require.NoError(t, f.Apply(flagset))
	}
	ctx := cli.NewContext(app, flagset, nil)

	// no flags defaults to DATABASE_URL
	u, err := getDatabaseURL(ctx)
	require.NoError(t, err)
	require.Equal(t, "foo://example.org/one", u.String())

	// --env overwrites DATABASE_URL
	require.NoError(t, ctx.Set("env", "CUSTOM_URL"))
	u, err = getDatabaseURL(ctx)
	require.NoError(t, err)
	require.Equal(t, "foo://example.org/two", u.String())

	// --url takes precedence over preceding two options
	require.NoError(t, ctx.Set("url", "foo://example.org/three"))
	u, err = getDatabaseURL(ctx)
	require.NoError(t, err)
	require.Equal(t, "foo://example.org/three", u.String())
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
