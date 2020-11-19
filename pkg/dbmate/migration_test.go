package dbmate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMigrationContents(t *testing.T) {
	// It supports the typical use case.
	migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;`

	up, down, err := parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	// It does not require space between the '--' and 'migrate'
	migration = `
--migrate:up
create table users (id serial, name text);

--migrate:down
drop table users;
`

	up, down, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "--migrate:up\ncreate table users (id serial, name text);\n\n", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "--migrate:down\ndrop table users;\n", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	// It is acceptable for down to be defined before up
	migration = `-- migrate:down
drop table users;
-- migrate:up
create table users (id serial, name text);
`

	up, down, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;\n", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	// It supports turning transactions off for a given migration block,
	// e.g., the below would not work in Postgres inside a transaction.
	// It also supports omitting the down block.
	migration = `-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
`

	up, down, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up transaction:false\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\n", up.Contents)
	require.Equal(t, false, up.Options.Transaction())

	require.Equal(t, "", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	// It does *not* support omitting the up block.
	migration = `-- migrate:down
drop table users;
`

	_, _, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate requires each migration to define an up bock with '-- migrate:up'", err.Error())

	// It allows leading comments and whitespace preceding the migrate blocks
	migration = `
-- This migration creates the users table.
-- It'll drop it in the event of a rollback.

-- migrate:up
create table users (id serial, name text);

-- migrate:down
drop table users;
`

	up, down, err = parseMigrationContents(migration)
	require.Nil(t, err)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n\n", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;\n", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

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

	_, _, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate does not support statements defined outside of the '-- migrate:up' or '-- migrate:down' blocks", err.Error())

	// It requires an at least an up block
	migration = `
ALTER TABLE users
ADD COLUMN status status_type DEFAULT 'active';
`

	_, _, err = parseMigrationContents(migration)
	require.NotNil(t, err)
	require.Equal(t, "dbmate requires each migration to define an up bock with '-- migrate:up'", err.Error())
}
