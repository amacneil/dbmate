package main

import (
	"flag"
	"github.com/codegangsta/cli"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

var stubsDir string

func testContext(t *testing.T, u *url.URL) *cli.Context {
	var err error
	if stubsDir == "" {
		stubsDir, err = filepath.Abs("./stubs")
		require.Nil(t, err)
	}

	err = os.Chdir(stubsDir)
	require.Nil(t, err)

	err = os.Setenv("DATABASE_URL", u.String())
	require.Nil(t, err)

	app := NewApp()
	flagset := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	for _, f := range app.Flags {
		f.Apply(flagset)
	}

	return cli.NewContext(app, flagset, nil)
}

func testURLs(t *testing.T) []*url.URL {
	return []*url.URL{
		postgresTestURL(t),
		mySQLTestURL(t),
		sqliteTestURL(t),
	}
}

func TestGetDatabaseUrl(t *testing.T) {
	envURL, err := url.Parse("foo://example.org/db")
	require.Nil(t, err)
	ctx := testContext(t, envURL)

	u, err := GetDatabaseURL(ctx)
	require.Nil(t, err)

	require.Equal(t, "foo", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}

func testMigrateCommandURL(t *testing.T, u *url.URL) {
	ctx := testContext(t, u)

	// drop and recreate database
	err := DropCommand(ctx)
	require.Nil(t, err)
	err = CreateCommand(ctx)
	require.Nil(t, err)

	// migrate
	err = MigrateCommand(ctx)
	require.Nil(t, err)

	// verify results
	db, err := GetDriverOpen(u)
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

func TestMigrateCommand(t *testing.T) {
	for _, u := range testURLs(t) {
		testMigrateCommandURL(t, u)
	}
}

func testUpCommandURL(t *testing.T, u *url.URL) {
	ctx := testContext(t, u)

	// drop database
	err := DropCommand(ctx)
	require.Nil(t, err)

	// create and migrate
	err = UpCommand(ctx)
	require.Nil(t, err)

	// verify results
	db, err := GetDriverOpen(u)
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
	for _, u := range testURLs(t) {
		testUpCommandURL(t, u)
	}
}

func testRollbackCommandURL(t *testing.T, u *url.URL) {
	ctx := testContext(t, u)

	// drop, recreate, and migrate database
	err := DropCommand(ctx)
	require.Nil(t, err)
	err = CreateCommand(ctx)
	require.Nil(t, err)
	err = MigrateCommand(ctx)
	require.Nil(t, err)

	// verify migration
	db, err := GetDriverOpen(u)
	require.Nil(t, err)
	defer mustClose(db)

	count := 0
	err = db.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	// rollback
	err = RollbackCommand(ctx)
	require.Nil(t, err)

	// verify rollback
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	err = db.QueryRow("select count(*) from users").Scan(&count)
	require.NotNil(t, err)
	require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())
}

func TestRollbackCommand(t *testing.T) {
	for _, u := range testURLs(t) {
		testRollbackCommandURL(t, u)
	}
}
