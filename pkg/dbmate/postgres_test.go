package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func postgresTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("postgres://postgres:postgres@postgres/dbmate?sslmode=disable")
	require.NoError(t, err)

	return u
}

func testPostgresDriver() *PostgresDriver {
	drv := &PostgresDriver{}
	drv.SetMigrationsTableName(DefaultMigrationsTableName)

	return drv
}

func prepTestPostgresDB(t *testing.T, u *url.URL) *sql.DB {
	drv := testPostgresDriver()

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// connect database
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)

	return db
}

func TestNormalizePostgresURL(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// defaults
		{"postgres:///foo", "postgres://localhost:5432/foo"},
		// support custom url params
		{"postgres://bob:secret@myhost:1234/foo?bar=baz", "postgres://bob:secret@myhost:1234/foo?bar=baz"},
		// support `host` and `port` via url params
		{"postgres://bob:secret@myhost:1234/foo?host=new&port=9999", "postgres://bob:secret@:9999/foo?host=new"},
		{"postgres://bob:secret@myhost:1234/foo?port=9999&bar=baz", "postgres://bob:secret@myhost:9999/foo?bar=baz"},
		// support unix sockets via `host` or `socket` param
		{"postgres://bob:secret@myhost:1234/foo?host=/var/run/postgresql", "postgres://bob:secret@:1234/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres://bob:secret@localhost/foo?socket=/var/run/postgresql", "postgres://bob:secret@:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
		{"postgres:///foo?socket=/var/run/postgresql", "postgres://:5432/foo?host=%2Fvar%2Frun%2Fpostgresql"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := normalizePostgresURL(u).String()
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestNormalizePostgresURLForDump(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		// defaults
		{"postgres:///foo", []string{"postgres://localhost:5432/foo"}},
		// support single schema
		{"postgres:///foo?search_path=foo", []string{"--schema", "foo", "postgres://localhost:5432/foo"}},
		// support multiple schemas
		{"postgres:///foo?search_path=foo,public", []string{"--schema", "foo", "--schema", "public", "postgres://localhost:5432/foo"}},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := normalizePostgresURLForDump(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestPostgresCreateDropDatabase(t *testing.T) {
	drv := testPostgresDriver()
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("postgres", u.String())
		require.NoError(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.NoError(t, err)
	}()

	// drop the database
	err = drv.DropDatabase(u)
	require.NoError(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("postgres", u.String())
		require.NoError(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.Error(t, err)
		require.Equal(t, "pq: database \"dbmate\" does not exist", err.Error())
	}()
}

func TestPostgresDumpSchema(t *testing.T) {
	t.Run("default migrations table", func(t *testing.T) {
		drv := testPostgresDriver()
		u := postgresTestURL(t)

		// prepare database
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)
		err := drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)

		// insert migration
		err = drv.InsertMigration(db, "abc1")
		require.NoError(t, err)
		err = drv.InsertMigration(db, "abc2")
		require.NoError(t, err)

		// DumpSchema should return schema
		schema, err := drv.DumpSchema(u, db)
		require.NoError(t, err)
		require.Contains(t, string(schema), "CREATE TABLE public.schema_migrations")
		require.Contains(t, string(schema), "\n--\n"+
			"-- PostgreSQL database dump complete\n"+
			"--\n\n\n"+
			"--\n"+
			"-- Dbmate schema migrations\n"+
			"--\n\n"+
			"INSERT INTO public.schema_migrations (version) VALUES\n"+
			"    ('abc1'),\n"+
			"    ('abc2');\n")

		// DumpSchema should return error if command fails
		u.Path = "/fakedb"
		schema, err = drv.DumpSchema(u, db)
		require.Nil(t, schema)
		require.EqualError(t, err, "pg_dump: [archiver (db)] connection to database "+
			"\"fakedb\" failed: FATAL:  database \"fakedb\" does not exist")
	})

	t.Run("custom migrations table with schema", func(t *testing.T) {
		drv := testPostgresDriver()
		drv.SetMigrationsTableName("camelSchema.testMigrations")

		u := postgresTestURL(t)

		// prepare database
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)
		err := drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)

		// insert migration
		err = drv.InsertMigration(db, "abc1")
		require.NoError(t, err)
		err = drv.InsertMigration(db, "abc2")
		require.NoError(t, err)

		// DumpSchema should return schema
		schema, err := drv.DumpSchema(u, db)
		require.NoError(t, err)
		require.Contains(t, string(schema), "CREATE TABLE \"camelSchema\".\"testMigrations\"")
		require.Contains(t, string(schema), "\n--\n"+
			"-- PostgreSQL database dump complete\n"+
			"--\n\n\n"+
			"--\n"+
			"-- Dbmate schema migrations\n"+
			"--\n\n"+
			"INSERT INTO \"camelSchema\".\"testMigrations\" (version) VALUES\n"+
			"    ('abc1'),\n"+
			"    ('abc2');\n")
	})
}

func TestPostgresDatabaseExists(t *testing.T) {
	drv := testPostgresDriver()
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestPostgresDatabaseExists_Error(t *testing.T) {
	drv := testPostgresDriver()
	u := postgresTestURL(t)
	u.User = url.User("invalid")

	exists, err := drv.DatabaseExists(u)
	require.Error(t, err)
	require.Equal(t, "pq: password authentication failed for user \"invalid\"", err.Error())
	require.Equal(t, false, exists)
}

func TestPostgresCreateMigrationsTable(t *testing.T) {
	t.Run("default schema", func(t *testing.T) {
		drv := testPostgresDriver()
		u := postgresTestURL(t)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"public.schema_migrations\" does not exist", err.Error())

		// create table
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from public.schema_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)
	})

	t.Run("custom search path", func(t *testing.T) {
		drv := testPostgresDriver()
		drv.SetMigrationsTableName("testMigrations")

		u, err := url.Parse(postgresTestURL(t).String() + "&search_path=camelFoo")
		require.NoError(t, err)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// delete schema
		_, err = db.Exec("drop schema if exists \"camelFoo\"")
		require.NoError(t, err)

		// drop any testMigrations table in public schema
		_, err = db.Exec("drop table if exists public.\"testMigrations\"")
		require.NoError(t, err)

		// migrations table should not exist in either schema
		count := 0
		err = db.QueryRow("select count(*) from \"camelFoo\".\"testMigrations\"").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"camelFoo.testMigrations\" does not exist", err.Error())
		err = db.QueryRow("select count(*) from public.\"testMigrations\"").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"public.testMigrations\" does not exist", err.Error())

		// create table
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)

		// camelFoo schema should be created, and migrations table should exist only in camelFoo schema
		err = db.QueryRow("select count(*) from \"camelFoo\".\"testMigrations\"").Scan(&count)
		require.NoError(t, err)
		err = db.QueryRow("select count(*) from public.\"testMigrations\"").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"public.testMigrations\" does not exist", err.Error())

		// create table should be idempotent
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)
	})

	t.Run("custom schema", func(t *testing.T) {
		drv := testPostgresDriver()
		drv.SetMigrationsTableName("camelSchema.testMigrations")

		u, err := url.Parse(postgresTestURL(t).String() + "&search_path=foo")
		require.NoError(t, err)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// delete schemas
		_, err = db.Exec("drop schema if exists foo")
		require.NoError(t, err)
		_, err = db.Exec("drop schema if exists \"camelSchema\"")
		require.NoError(t, err)

		// migrations table should not exist
		count := 0
		err = db.QueryRow("select count(*) from \"camelSchema\".\"testMigrations\"").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"camelSchema.testMigrations\" does not exist", err.Error())

		// create table
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)

		// camelSchema should be created, and testMigrations table should exist
		err = db.QueryRow("select count(*) from \"camelSchema\".\"testMigrations\"").Scan(&count)
		require.NoError(t, err)
		// testMigrations table should not exist in foo schema because
		// schema specified with migrations table name takes priority over search path
		err = db.QueryRow("select count(*) from foo.\"testMigrations\"").Scan(&count)
		require.Error(t, err)
		require.Equal(t, "pq: relation \"foo.testMigrations\" does not exist", err.Error())

		// create table should be idempotent
		err = drv.CreateMigrationsTable(u, db)
		require.NoError(t, err)
	})
}

func TestPostgresSelectMigrations(t *testing.T) {
	drv := testPostgresDriver()
	drv.SetMigrationsTableName("test_migrations")

	u := postgresTestURL(t)
	db := prepTestPostgresDB(t, u)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(u, db)
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
	drv := testPostgresDriver()
	drv.SetMigrationsTableName("test_migrations")

	u := postgresTestURL(t)
	db := prepTestPostgresDB(t, u)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(u, db)
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
	drv := testPostgresDriver()
	drv.SetMigrationsTableName("test_migrations")

	u := postgresTestURL(t)
	db := prepTestPostgresDB(t, u)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(u, db)
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
	drv := testPostgresDriver()
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// ping database
	err = drv.Ping(u)
	require.NoError(t, err)

	// ping invalid host should return error
	u.Host = "postgres:404"
	err = drv.Ping(u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connect: connection refused")
}

func TestPostgresQuotedMigrationsTableName(t *testing.T) {
	drv := testPostgresDriver()

	t.Run("default schema", func(t *testing.T) {
		u := postgresTestURL(t)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.schema_migrations", name)
	})

	t.Run("custom schema", func(t *testing.T) {
		u, err := url.Parse(postgresTestURL(t).String() + "&search_path=foo,bar,public")
		require.NoError(t, err)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// if "foo" schema does not exist, current schema should be "public"
		_, err = db.Exec("drop schema if exists foo")
		require.NoError(t, err)
		_, err = db.Exec("drop schema if exists bar")
		require.NoError(t, err)
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.schema_migrations", name)

		// if "foo" schema exists, it should be used
		_, err = db.Exec("create schema foo")
		require.NoError(t, err)
		name, err = drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "foo.schema_migrations", name)
	})

	t.Run("no schema", func(t *testing.T) {
		u := postgresTestURL(t)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// this is an unlikely edge case, but if for some reason there is
		// no current schema then we should default to "public"
		_, err := db.Exec("select pg_catalog.set_config('search_path', '', false)")
		require.NoError(t, err)

		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.schema_migrations", name)
	})

	t.Run("custom table name", func(t *testing.T) {
		u := postgresTestURL(t)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		drv.SetMigrationsTableName("simple_name")
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.simple_name", name)
	})

	t.Run("custom table name quoted", func(t *testing.T) {
		u := postgresTestURL(t)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		// this table name will need quoting
		drv.SetMigrationsTableName("camelCase")
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "public.\"camelCase\"", name)
	})

	t.Run("custom table name with custom schema", func(t *testing.T) {
		u, err := url.Parse(postgresTestURL(t).String() + "&search_path=foo")
		require.NoError(t, err)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		_, err = db.Exec("create schema if not exists foo")
		require.NoError(t, err)

		drv.SetMigrationsTableName("simple_name")
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "foo.simple_name", name)
	})

	t.Run("custom table name overrides schema", func(t *testing.T) {
		u, err := url.Parse(postgresTestURL(t).String() + "&search_path=foo")
		require.NoError(t, err)
		db := prepTestPostgresDB(t, u)
		defer mustClose(db)

		_, err = db.Exec("create schema if not exists foo")
		require.NoError(t, err)
		_, err = db.Exec("create schema if not exists bar")
		require.NoError(t, err)

		// if schema is specified as part of table name, it should override search_path
		drv.SetMigrationsTableName("bar.simple_name")
		name, err := drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "bar.simple_name", name)

		// schema and table name should be quoted if necessary
		drv.SetMigrationsTableName("barName.camelTable")
		name, err = drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "\"barName\".\"camelTable\"", name)

		// more than 2 components is unexpected but we will quote and pass it along anyway
		drv.SetMigrationsTableName("whyWould.i.doThis")
		name, err = drv.quotedMigrationsTableName(db)
		require.NoError(t, err)
		require.Equal(t, "\"whyWould\".i.\"doThis\"", name)
	})
}
