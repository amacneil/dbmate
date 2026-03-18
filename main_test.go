package main

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
)

func TestGetDatabaseUrl(t *testing.T) {
	// set environment variables
	t.Setenv("DATABASE_URL", "foo://example.org/one")
	t.Setenv("CUSTOM_URL", "foo://example.org/two")

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

func TestLoadEnvFiles(t *testing.T) {
	setup := func(t *testing.T) {
		env := os.Environ()

		// t.Chdir automatically restores the original directory when the test completes
		t.Chdir("fixtures/loadEnvFiles")

		// Clear environment after t.Chdir because t.Chdir sets PWD
		os.Clearenv()

		// Restore environment variables when test completes
		t.Cleanup(func() {
			os.Clearenv()

			for _, e := range env {
				pair := strings.SplitN(e, "=", 2)
				t.Setenv(pair[0], pair[1])
			}
		})
	}

	t.Run("default file is .env", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{})
		require.NoError(t, err)

		require.Equal(t, 1, len(os.Environ()))
		require.Equal(t, "default", os.Getenv("TEST_DOTENV"))
	})

	t.Run("valid file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "first.txt"})
		require.NoError(t, err)
		require.Equal(t, 1, len(os.Environ()))
		require.Equal(t, "one", os.Getenv("FIRST"))
	})

	t.Run("two valid files", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "first.txt", "--env-file", "second.txt"})
		require.NoError(t, err)
		require.Equal(t, 2, len(os.Environ()))
		require.Equal(t, "one", os.Getenv("FIRST"))
		require.Equal(t, "two", os.Getenv("SECOND"))
	})

	t.Run("nonexistent file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "nonexistent.txt"})
		require.NoError(t, err)
		require.Equal(t, 0, len(os.Environ()))
	})

	t.Run("no overload", func(t *testing.T) {
		setup(t)

		// we do not load values over existing values
		t.Setenv("FIRST", "not one")

		err := loadEnvFiles([]string{"--env-file", "first.txt"})
		require.NoError(t, err)
		require.Equal(t, 1, len(os.Environ()))
		require.Equal(t, "not one", os.Getenv("FIRST"))
	})

	t.Run("invalid file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "invalid.txt"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected character \"\\n\" in variable name near \"INVALID ENV FILE\\n\"")
		require.Equal(t, 0, len(os.Environ()))
	})

	t.Run("invalid file followed by a valid file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "invalid.txt", "--env-file", "first.txt"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected character \"\\n\" in variable name near \"INVALID ENV FILE\\n\"")
		require.Equal(t, 0, len(os.Environ()))
	})

	t.Run("valid file followed by an invalid file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "first.txt", "--env-file", "invalid.txt"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected character \"\\n\" in variable name near \"INVALID ENV FILE\\n\"")
		require.Equal(t, 1, len(os.Environ()))
		require.Equal(t, "one", os.Getenv("FIRST"))
	})

	t.Run("valid file followed by an invalid file followed by a valid file", func(t *testing.T) {
		setup(t)

		err := loadEnvFiles([]string{"--env-file", "first.txt", "--env-file", "invalid.txt", "--env-file", "second.txt"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected character \"\\n\" in variable name near \"INVALID ENV FILE\\n\"")
		// files after an invalid file should not get loaded
		require.Equal(t, 1, len(os.Environ()))
		require.Equal(t, "one", os.Getenv("FIRST"))
	})
}

func TestConfigureDB_Driver(t *testing.T) {
	var configuredDB *dbmate.DB

	app := NewApp()
	app.Commands = []*cli.Command{
		{
			Name: "test-config",
			Action: func(c *cli.Context) error {
				var err error
				configuredDB, err = configureDB(c)
				return err
			},
		},
	}

	t.Run("default is empty", func(t *testing.T) {
		configuredDB = nil
		err := app.Run([]string{"dbmate", "test-config"})
		require.NoError(t, err)
		require.Empty(t, configuredDB.DriverName)
	})

	t.Run("from env variable", func(t *testing.T) {
		configuredDB = nil
		t.Setenv("DBMATE_DRIVER", "postgres")

		err := app.Run([]string{"dbmate", "test-config"})
		require.NoError(t, err)
		require.Equal(t, "postgres", configuredDB.DriverName)
	})

	t.Run("flag overrides env variable", func(t *testing.T) {
		configuredDB = nil
		t.Setenv("DBMATE_DRIVER", "postgres")

		err := app.Run([]string{"dbmate", "--driver", "clickhouse", "test-config"})
		require.NoError(t, err)
		require.Equal(t, "clickhouse", configuredDB.DriverName)
	})
}
