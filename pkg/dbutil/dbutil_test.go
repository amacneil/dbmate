package dbutil_test

import (
	"database/sql"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	_ "github.com/mattn/go-sqlite3" // database/sql driver
	"github.com/stretchr/testify/require"
)

func TestDatabaseName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "foo://host/dbname?query")
		name := dbutil.DatabaseName(u)
		require.Equal(t, "dbname", name)
	})

	t.Run("empty", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "foo://host")
		name := dbutil.DatabaseName(u)
		require.Equal(t, "", name)
	})
}

func TestTrimLeadingSQLComments(t *testing.T) {
	t.Run("basic comments", func(t *testing.T) {
		in := "--\n" +
			"-- foo\n\n" +
			"-- bar\n\n" +
			"real stuff\n" +
			"-- end\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "real stuff\n-- end\n", string(out))
	})

	t.Run("restrict header", func(t *testing.T) {
		in := "\\restrict abc123\n" +
			"--\n" +
			"-- PostgreSQL database dump\n" +
			"--\n\n" +
			"CREATE TABLE users (id int);\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "CREATE TABLE users (id int);\n", string(out))
	})

	t.Run("multiple backslash commands", func(t *testing.T) {
		in := "\\restrict key123\n" +
			"\\set VERBOSITY terse\n" +
			"--\n" +
			"-- Database dump\n" +
			"--\n\n" +
			"CREATE SCHEMA public;\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "CREATE SCHEMA public;\n", string(out))
	})

	t.Run("mixed headers with restrict", func(t *testing.T) {
		in := "\\restrict secure_key\n" +
			"--\n" +
			"-- PostgreSQL database dump\n" +
			"-- Dumped from database version 16.1\n" +
			"--\n\n" +
			"SET statement_timeout = 0;\n" +
			"CREATE TABLE test (id int);\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "SET statement_timeout = 0;\nCREATE TABLE test (id int);\n", string(out))
	})

	t.Run("backslash in content should not be stripped", func(t *testing.T) {
		in := "--\n" +
			"-- Header\n" +
			"--\n\n" +
			"CREATE TABLE test (path text);\n" +
			"INSERT INTO test VALUES ('C:\\\\Windows\\\\System32');\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "CREATE TABLE test (path text);\nINSERT INTO test VALUES ('C:\\\\Windows\\\\System32');\n", string(out))
	})

	t.Run("unrestrict footer", func(t *testing.T) {
		in := "\\restrict key123\n" +
			"--\n" +
			"-- PostgreSQL database dump\n" +
			"--\n\n" +
			"CREATE TABLE users (id int);\n" +
			"INSERT INTO users VALUES (1);\n" +
			"\\unrestrict\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "CREATE TABLE users (id int);\nINSERT INTO users VALUES (1);\n", string(out))
	})

	t.Run("trailing whitespace and comments", func(t *testing.T) {
		in := "\\restrict abc\n" +
			"-- Header\n" +
			"CREATE SCHEMA public;\n" +
			"CREATE TABLE test (id int);\n" +
			"\n" +
			"-- Footer comment\n" +
			"\\unrestrict\n" +
			"\n\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		// Footer SQL comments are preserved, only meta-commands and whitespace are stripped
		require.Equal(t, "CREATE SCHEMA public;\nCREATE TABLE test (id int);\n\n-- Footer comment\n", string(out))
	})

	t.Run("complete pg_dump style with restrict headers", func(t *testing.T) {
		in := "\\restrict secure_key_123\n" +
			"--\n" +
			"-- PostgreSQL database dump\n" +
			"-- Dumped from database version 17.6\n" +
			"-- Dumped by pg_dump version 17.6\n" +
			"--\n\n" +
			"SET statement_timeout = 0;\n" +
			"CREATE SCHEMA public;\n" +
			"CREATE TABLE users (id serial, name text);\n" +
			"\n" +
			"--\n" +
			"-- PostgreSQL database dump complete\n" +
			"--\n" +
			"\\unrestrict\n" +
			"\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		// SQL comments within content are preserved, only leading/trailing meta-commands are stripped
		expected := "SET statement_timeout = 0;\nCREATE SCHEMA public;\nCREATE TABLE users (id serial, name text);\n\n--\n-- PostgreSQL database dump complete\n--\n"
		require.Equal(t, expected, string(out))
	})

	t.Run("empty content between headers", func(t *testing.T) {
		in := "\\restrict key\n" +
			"-- Header\n" +
			"\n\n" +
			"-- Footer\n" +
			"\\unrestrict\n"
		out, err := dbutil.TrimLeadingSQLComments([]byte(in))
		require.NoError(t, err)
		require.Equal(t, "", string(out))
	})
}

// connect to in-memory sqlite database for testing
const sqliteMemoryDB = "file:dbutil.sqlite3?mode=memory&cache=shared"

func TestQueryColumn(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteMemoryDB)
	require.NoError(t, err)

	val, err := dbutil.QueryColumn(db, "select 'foo_' || val from (select ? as val union select ?)",
		"hi", "there")
	require.NoError(t, err)
	require.Equal(t, []string{"foo_hi", "foo_there"}, val)
}

func TestQueryValue(t *testing.T) {
	db, err := sql.Open("sqlite3", sqliteMemoryDB)
	require.NoError(t, err)

	val, err := dbutil.QueryValue(db, "select $1 + $2", "5", 2)
	require.NoError(t, err)
	require.Equal(t, "7", val)
}
