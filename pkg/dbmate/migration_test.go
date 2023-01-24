package dbmate

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	fs := fstest.MapFS{
		"bar/123_foo.sql": {
			Data: []byte(`-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;
`),
		},
	}

	migration := &Migration{
		Applied:  false,
		FileName: "123_foo.sql",
		FilePath: "bar/123_foo.sql",
		FS:       fs,
		Version:  "123",
	}

	parsed, err := migration.Parse()
	require.Nil(t, err)
	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
	require.True(t, parsed.UpOptions.Transaction())
	require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
	require.True(t, parsed.DownOptions.Transaction())
}

func TestParseMigrationContents(t *testing.T) {
	// It supports the typical use case.
	migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;`

	parsed, err := parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
	require.Equal(t, true, parsed.UpOptions.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", parsed.Down)
	require.Equal(t, true, parsed.DownOptions.Transaction())

	// It does not require space between the '--' and 'migrate'
	migration = `
--migrate:up
create table users (id serial, name text);

--migrate:down
drop table users;
`

	parsed, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "--migrate:up\ncreate table users (id serial, name text);\n\n", parsed.Up)
	require.Equal(t, true, parsed.UpOptions.Transaction())

	require.Equal(t, "--migrate:down\ndrop table users;\n", parsed.Down)
	require.Equal(t, true, parsed.DownOptions.Transaction())

	// It is acceptable for down to be defined before up
	migration = `-- migrate:down
drop table users;
-- migrate:up
create table users (id serial, name text);
`

	parsed, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
	require.Equal(t, true, parsed.UpOptions.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
	require.Equal(t, true, parsed.DownOptions.Transaction())

	// It supports turning transactions off for a given migration block,
	// e.g., the below would not work in Postgres inside a transaction.
	// It also supports omitting the down block.
	migration = `-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
`

	parsed, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up transaction:false\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\n", parsed.Up)
	require.Equal(t, false, parsed.UpOptions.Transaction())

	require.Equal(t, "", parsed.Down)
	require.Equal(t, true, parsed.DownOptions.Transaction())

	// It does *not* support omitting the up block.
	migration = `-- migrate:down
drop table users;
`

	_, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate requires each migration to define an up block with '-- migrate:up'", err.Error())

	// It allows leading comments and whitespace preceding the migrate blocks
	migration = `
-- This migration creates the users table.
-- It'll drop it in the event of a rollback.

-- migrate:up
create table users (id serial, name text);

-- migrate:down
drop table users;
`

	parsed, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n\n", parsed.Up)
	require.Equal(t, true, parsed.UpOptions.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
	require.Equal(t, true, parsed.DownOptions.Transaction())

	// It does *not* allow arbitrary statements preceding the migrate blocks
	migration = `
-- create status_type
CREATE TYPE status_type AS ENUM ('active', 'inactive');

-- migrate:up
ALTER TABLE users
ADD COLUMN status status_type DEFAULT 'active';

-- migrate:down
ALTER TABLE users
DROP COLUMN status;
`

	_, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate does not support statements defined outside of the '-- migrate:up' or '-- migrate:down' blocks", err.Error())

	// It requires an at least an up block
	migration = `
ALTER TABLE users
ADD COLUMN status status_type DEFAULT 'active';
`

	_, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate requires each migration to define an up block with '-- migrate:up'", err.Error())
}
