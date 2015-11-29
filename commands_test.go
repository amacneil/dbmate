package main_test

import (
	"database/sql"
	"flag"
	"github.com/adrianmacneil/dbmate"
	"github.com/codegangsta/cli"
	"github.com/stretchr/testify/require"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

var stubsDir string

func testContext(t *testing.T) *cli.Context {
	var err error
	if stubsDir == "" {
		stubsDir, err = filepath.Abs("./stubs")
		require.Nil(t, err)
	}

	err = os.Chdir(stubsDir)
	require.Nil(t, err)

	u := testURL(t)
	err = os.Setenv("DATABASE_URL", u.String())
	require.Nil(t, err)

	app := main.NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		f.Apply(flagset)
	}

	return cli.NewContext(app, flagset, nil)
}

func testURL(t *testing.T) *url.URL {
	str := os.Getenv("POSTGRES_PORT")
	require.NotEmpty(t, str, "missing POSTGRES_PORT environment variable")

	u, err := url.Parse(str)
	require.Nil(t, err)

	u.Scheme = "postgres"
	u.User = url.User("postgres")
	u.Path = "/dbmate"
	u.RawQuery = "sslmode=disable"

	return u
}

func mustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
}

func TestGetDatabaseUrl(t *testing.T) {
	ctx := testContext(t)

	err := os.Setenv("DATABASE_URL", "postgres://example.org/db")
	require.Nil(t, err)

	u, err := main.GetDatabaseURL(ctx)
	require.Nil(t, err)

	require.Equal(t, "postgres", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}

func TestMigrateCommand(t *testing.T) {
	ctx := testContext(t)

	// drop and recreate database
	err := main.DropCommand(ctx)
	require.Nil(t, err)
	err = main.CreateCommand(ctx)
	require.Nil(t, err)

	// migrate
	err = main.MigrateCommand(ctx)
	require.Nil(t, err)

	// verify results
	u := testURL(t)
	db, err := sql.Open("postgres", u.String())
	require.Nil(t, err)
	defer mustClose(db)

	count := 0
	err = db.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	err = db.QueryRow("select count(*) from users").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestUpCommand(t *testing.T) {
	ctx := testContext(t)

	// drop database
	err := main.DropCommand(ctx)
	require.Nil(t, err)

	// create and migrate
	err = main.UpCommand(ctx)
	require.Nil(t, err)

	// verify results
	u := testURL(t)
	db, err := sql.Open("postgres", u.String())
	require.Nil(t, err)
	defer mustClose(db)

	count := 0
	err = db.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	err = db.QueryRow("select count(*) from users").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestRollbackCommand(t *testing.T) {
	ctx := testContext(t)

	// drop, recreate, and migrate database
	err := main.DropCommand(ctx)
	require.Nil(t, err)
	err = main.CreateCommand(ctx)
	require.Nil(t, err)
	err = main.MigrateCommand(ctx)
	require.Nil(t, err)

	// verify migration
	u := testURL(t)
	db, err := sql.Open("postgres", u.String())
	require.Nil(t, err)
	defer mustClose(db)

	count := 0
	err = db.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	// rollback
	err = main.RollbackCommand(ctx)
	require.Nil(t, err)

	// verify rollback
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	err = db.QueryRow("select count(*) from users").Scan(&count)
	require.Equal(t, "pq: relation \"users\" does not exist", err.Error())
}
