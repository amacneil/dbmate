package postgres

import (
	"database/sql"
	"fmt"
	"net/url"
	"runtime"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testPostgresDriver(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "POSTGRES_TEST_URL")
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func testRedshiftDriver(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "REDSHIFT_TEST_URL")
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func testSpannerPostgresDriver(t *testing.T) *Driver {
	// URL to the spanner pgadapter, or a locally-running spanner emulator with the pgadapter
	u := dbtest.GetenvURLOrSkip(t, "SPANNER_POSTGRES_TEST_URL")
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestPostgresDB(t *testing.T) *sql.DB {
	drv := testPostgresDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// connect database
	db, err := sql.Open("postgres", connectionString(drv.databaseURL))
	require.NoError(t, err)

	return db
}

func prepRedshiftTestDB(t *testing.T, drv *Driver) *sql.DB {
	// connect database
	db, err := sql.Open("postgres", connectionString(drv.databaseURL))
	require.NoError(t, err)

	_, migrationsTable, err := drv.quotedMigrationsTableNameParts(db)
	if err != nil {
		t.Error(err)
	}

	_, err = db.Exec(fmt.Sprintf("drop table if exists %s", migrationsTable))
	require.NoError(t, err)

	return db
}

func prepTestSpannerPostgresDB(t *testing.T, drv *Driver) *sql.DB {
	// Spanner doesn't allow running `drop database`, so we just drop the migrations
	// table instead
	db, err := sql.Open("postgres", connectionString(drv.databaseURL))
	require.NoError(t, err)

	_, migrationsTable, err := drv.quotedMigrationsTableNameParts(db)
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("drop table if exists %s", migrationsTable))
	require.NoError(t, err)

	return db
}

func TestGetDriver(t *testing.T) {
	db := dbmate.New(dbtest.MustParseURL(t, "postgres://"))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestPgDumpVersionRegexp(t *testing.T) {
	cases := []struct {
		input         string
		expectedMajor int
		expectedMinor int
	}{
		{"pg_dump (PostgreSQL) 17.6 (Debian 17.6-1.pgdg120+1)", 17, 6},
		{"pg_dump (PostgreSQL) 16.0", 16, 0},
		{"pg_dump (PostgreSQL) 15.12", 15, 12},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			matches := pgDumpVersionRegexp.FindStringSubmatch(c.input)
			require.Len(t, matches, 3)
			require.Equal(t, fmt.Sprintf("%d", c.expectedMajor), matches[1])
			require.Equal(t, fmt.Sprintf("%d", c.expectedMinor), matches[2])
		})
	}
}

func TestPgDumpVersionSupportsRestrictKey(t *testing.T) {
	cases := []struct {
		name     string
		version  *pgDumpVersion
		expected bool
	}{
		{"nil version", nil, false},
		{"PostgreSQL 15.13", &pgDumpVersion{major: 15, minor: 13}, false},
		{"PostgreSQL 15.14", &pgDumpVersion{major: 15, minor: 14}, true},
		{"PostgreSQL 16.0", &pgDumpVersion{major: 16, minor: 0}, false},
		{"PostgreSQL 16.9", &pgDumpVersion{major: 16, minor: 9}, false},
		{"PostgreSQL 16.10", &pgDumpVersion{major: 16, minor: 10}, true},
		{"PostgreSQL 17.0", &pgDumpVersion{major: 17, minor: 0}, false},
		{"PostgreSQL 17.5", &pgDumpVersion{major: 17, minor: 5}, false},
		{"PostgreSQL 17.6", &pgDumpVersion{major: 17, minor: 6}, true},
		{"PostgreSQL 17.7", &pgDumpVersion{major: 17, minor: 7}, true},
		{"PostgreSQL 18.0", &pgDumpVersion{major: 18, minor: 0}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := c.version.supportsRestrictKey()
			require.Equal(t, c.expected, result)
		})
	}
}

func defaultConnString() string {
	switch runtime.GOOS {
	case "linux":
		return "postgres://:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"
	case "darwin", "freebsd", "dragonfly", "openbsd", "netbsd":
		return "postgres://:5432/foo?host=%2Ftmp"
	default:
		return "postgres://localhost:5432/foo"
	}
}

func TestConnectionString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// defaults
		{"postgres:///foo", defaultConnString()},
		// support custom url params
		{"postgres://bob:secret@myhost:1234/foo?bar=baz", "postgres://bob:secret@myhost:1234/foo?bar=baz"},
		// support `host` and `port` via url params
		{"postgres://bob:secret@myhost:1234/foo?host=new&port=9999", "postgres://bob:secret@:9999/foo?host=new"},
		{"postgres://bob:secret@myhost:1234/foo?port=9999&bar=baz", "postgres://bob:secret@myhost:9999/foo?bar=baz"},
		// support unix sockets via `host` or `socket` param
		{"postgres://bob:secret@myhost:1234/foo?host=/var/run/postgresql", "postgres://bob:secret@:1234/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres://bob:secret@localhost/foo?socket=/var/run/postgresql", "postgres://bob:secret@:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres:///foo?socket=/var/run/postgresql", "postgres://:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres://bob:secret@/foo?socket=/var/run/postgresql", "postgres://bob:secret@:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres://bob:secret@/foo?host=/var/run/postgresql", "postgres://bob:secret@:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		// redshift default port is 5439, not 5432
		{"redshift://myhost/foo", "postgres://myhost:5439/foo"},
		{"spanner-postgres://myhost/foo", "postgres://myhost:5432/foo"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := connectionString(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestConnectionArgsForDump(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		// defaults
		{"postgres:///foo", []string{defaultConnString()}},
		// support single schema
		{"postgres:///foo?search_path=foo", []string{"--schema", "foo", defaultConnString()}},
		// support multiple schemas
		{"postgres:///foo?search_path=foo,public", []string{"--schema", "foo", "--schema", "public", defaultConnString()}},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := connectionArgsForDump(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestPostgresCreateDropDatabase(t *testing.T) {
	drv := testPostgresDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("postgres", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.NoError(t, err)
	}()

	// drop the database
	err = drv.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("postgres", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.ErrorContains(t, err, "pq: database \"dbmate_test\" does not exist")
	}()
}

func TestPostgresDumpSchema(t *testing.T) {
	t.Run("default migrations table", func(t *testing.T) {
		drv := testPostgresDriver(t)

		// prepare database
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)
		err := drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// insert migration
		err = drv.InsertMigration(db, "abc1")
		require.NoError(t, err)
		err = drv.InsertMigration(db, "abc2")
		require.NoError(t, err)

		// DumpSchema should return schema
		schema, err := drv.DumpSchema(db)
		require.NoError(t, err)
		require.Contains(t, string(schema), "CREATE TABLE public.schema_migrations")
		require.Contains(t, string(schema), "\n--\n"+
			"-- PostgreSQL database dump complete\n"+
			"--\n\n")
		require.Contains(t, string(schema), "-- Dbmate schema migrations\n"+
			"--\n\n"+
			"INSERT INTO public.schema_migrations (version) VALUES\n"+
			"    ('abc1'),\n"+
			"    ('abc2');\n")

		// DumpSchema should return error if command fails
		drv.databaseURL.Path = "/fakedb"
		schema, err = drv.DumpSchema(db)
		require.Nil(t, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "database \"fakedb\" does not exist")
	})

	t.Run("custom migrations table with schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "camelSchema.testMigrations"

		// prepare database
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)
		err := drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// insert migration
		err = drv.InsertMigration(db, "abc1")
		require.NoError(t, err)
		err = drv.InsertMigration(db, "abc2")
		require.NoError(t, err)

		// DumpSchema should return schema
		schema, err := drv.DumpSchema(db)
		require.NoError(t, err)
		require.Contains(t, string(schema), "CREATE TABLE \"camelSchema\".\"testMigrations\"")
		require.Contains(t, string(schema), "\n--\n"+
			"-- PostgreSQL database dump complete\n"+
			"--\n\n")
		require.Contains(t, string(schema), "-- Dbmate schema migrations\n"+
			"--\n\n"+
			"INSERT INTO \"camelSchema\".\"testMigrations\" (version) VALUES\n"+
			"    ('abc1'),\n"+
			"    ('abc2');\n")
	})
}

func TestPostgresDatabaseExists(t *testing.T) {
	drv := testPostgresDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestPostgresDatabaseExists_Error(t *testing.T) {
	drv := testPostgresDriver(t)
	drv.databaseURL.User = url.User("invalid")

	exists, err := drv.DatabaseExists()
	require.ErrorContains(t, err, "pq: password authentication failed for user \"invalid\"")
	require.Equal(t, false, exists)
}

func TestPostgresCreateMigrationsTable(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"public.schema_migrations\" does not exist")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})

	t.Run("custom search path", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "testMigrations"

		u, err := url.Parse(drv.databaseURL.String() + "&search_path=camelFoo")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		// delete schema
		_, err = db.Exec("drop schema if exists \"camelFoo\"")
		require.NoError(t, err)

		// drop any testMigrations table in public schema
		_, err = db.Exec("drop table if exists public.\"testMigrations\"")
		require.NoError(t, err)

		// migrations table should not exist in either schema
		count := 0
		err = db.QueryRow("select count(*) from \"camelFoo\".\"testMigrations\"").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"camelFoo.testMigrations\" does not exist")
		err = db.QueryRow("select count(*) from public.\"testMigrations\"").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"public.testMigrations\" does not exist")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// camelFoo schema should be created, and migrations table should exist only in camelFoo schema
		err = db.QueryRow("select count(*) from \"camelFoo\".\"testMigrations\"").Scan(&count)
		require.NoError(t, err)
		err = db.QueryRow("select count(*) from public.\"testMigrations\"").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"public.testMigrations\" does not exist")

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})

	t.Run("custom schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "camelSchema.testMigrations"

		u, err := url.Parse(drv.databaseURL.String() + "&search_path=foo")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		// delete schemas
		_, err = db.Exec("drop schema if exists foo")
		require.NoError(t, err)
		_, err = db.Exec("drop schema if exists \"camelSchema\"")
		require.NoError(t, err)

		// migrations table should not exist
		count := 0
		err = db.QueryRow("select count(*) from \"camelSchema\".\"testMigrations\"").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"camelSchema.testMigrations\" does not exist")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// camelSchema should be created, and testMigrations table should exist
		err = db.QueryRow("select count(*) from \"camelSchema\".\"testMigrations\"").Scan(&count)
		require.NoError(t, err)
		// testMigrations table should not exist in foo schema because
		// schema specified with migrations table name takes priority over search path
		err = db.QueryRow("select count(*) from foo.\"testMigrations\"").Scan(&count)
		require.ErrorContains(t, err, "pq: relation \"foo.testMigrations\" does not exist")

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})
}

func TestRedshiftCreateMigrationsTable(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testRedshiftDriver(t)
		db := prepRedshiftTestDB(t, drv)
		defer dbutil.MustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.Error(t, err, "migrations table exists when it shouldn't")
		require.Equal(t, "pq: relation \"public.schema_migrations\" does not exist", err.Error())

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})
}

func TestSpannerPostgresCreateMigrationsTable(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testSpannerPostgresDriver(t)
		db := prepTestSpannerPostgresDB(t, drv)
		defer dbutil.MustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.Error(t, err, "migrations table exists when it shouldn't")
		require.ErrorContains(t, err, "pq: relation \"public.schema_migrations\" does not exist")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.NoError(t, err)
	})
}

func TestPostgresSelectMigrations(t *testing.T) {
	drv := testPostgresDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestPostgresDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into public.test_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.NoError(t, err)

	migrations, err := drv.SelectMigrations(db, -1)
	require.NoError(t, err)
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1)
	require.NoError(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
}

func TestPostgresInsertMigration(t *testing.T) {
	drv := testPostgresDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestPostgresDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from public.test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from public.test_migrations where version = 'abc1'").
		Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestPostgresDeleteMigration(t *testing.T) {
	drv := testPostgresDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestPostgresDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into public.test_migrations (version)
		values ('abc1'), ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from public.test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestPostgresPing(t *testing.T) {
	drv := testPostgresDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.NoError(t, err)

	// ping invalid host should return error
	drv.databaseURL.Host = "postgres:404"
	err = drv.Ping()
	require.ErrorContains(t, err, "connect: connection refused")
}

func TestPostgresQuotedMigrationsTableName(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.schema_migrations", name)
	})

	t.Run("custom schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		u, err := url.Parse(drv.databaseURL.String() + "&search_path=foo,bar,public")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		_, err = db.Exec("drop schema if exists foo")
		require.NoError(t, err)
		_, err = db.Exec("drop schema if exists bar")
		require.NoError(t, err)

		// should use first schema from search path
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "foo.schema_migrations", name)
	})

	t.Run("no schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		// this is an unlikely edge case, but if for some reason there is
		// no current schema then we should default to "public"
		_, err := db.Exec("select pg_catalog.set_config('search_path', '', false)")
		require.NoError(t, err)

		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.schema_migrations", name)
	})

	t.Run("custom table name", func(t *testing.T) {
		drv := testPostgresDriver(t)
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		drv.migrationsTableName = "simple_name"
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.simple_name", name)
	})

	t.Run("custom table name quoted", func(t *testing.T) {
		drv := testPostgresDriver(t)
		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		// this table name will need quoting
		drv.migrationsTableName = "camelCase"
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.\"camelCase\"", name)
	})

	t.Run("custom table name with custom schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		u, err := url.Parse(drv.databaseURL.String() + "&search_path=foo")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		_, err = db.Exec("create schema if not exists foo")
		require.NoError(t, err)

		drv.migrationsTableName = "simple_name"
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "foo.simple_name", name)
	})

	t.Run("custom table name overrides schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		u, err := url.Parse(drv.databaseURL.String() + "&search_path=foo")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		_, err = db.Exec("create schema if not exists foo")
		require.NoError(t, err)
		_, err = db.Exec("create schema if not exists bar")
		require.NoError(t, err)

		// if schema is specified as part of table name, it should override search_path
		drv.migrationsTableName = "bar.simple_name"
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "bar.simple_name", name)

		// schema and table name should be quoted if necessary
		drv.migrationsTableName = "barName.camelTable"
		name, err = drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "\"barName\".\"camelTable\"", name)

		// more than 2 components is unexpected but we will quote and pass it along anyway
		drv.migrationsTableName = "whyWould.i.doThis"
		name, err = drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "\"whyWould\".i.\"doThis\"", name)
	})
}

func TestPostgresMigrationsTableExists(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "test_migrations"

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, false, exists)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err = drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})

	t.Run("custom schema", func(t *testing.T) {
		drv := testPostgresDriver(t)
		u, err := url.Parse(drv.databaseURL.String() + "&search_path=foo")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})

	t.Run("custom schema with special chars", func(t *testing.T) {
		drv := testPostgresDriver(t)
		u, err := url.Parse(drv.databaseURL.String() + "&search_path=custom-schema")
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})

	t.Run("custom migrations table name containing schema with special chars", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "custom$schema.schema_migrations"
		u, err := url.Parse(drv.databaseURL.String())
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})

	t.Run("custom migrations table name containing table name with special chars", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "schema.custom#table#name"
		u, err := url.Parse(drv.databaseURL.String())
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})

	t.Run("custom migrations table name containing schema and table name with special chars", func(t *testing.T) {
		drv := testPostgresDriver(t)
		drv.migrationsTableName = "custom-schema.custom@table@name"
		u, err := url.Parse(drv.databaseURL.String())
		require.NoError(t, err)
		drv.databaseURL = u

		db := prepTestPostgresDB(t)
		defer dbutil.MustClose(db)

		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)
	})
}
