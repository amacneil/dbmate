package dbmate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMigrationContents(t *testing.T) {
	migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;`

	up, down := parseMigrationContents(migration)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	migration = `-- migrate:down
drop table users;
-- migrate:up
create table users (id serial, name text);
`

	up, down = parseMigrationContents(migration)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);", up.Contents)
	require.Equal(t, true, up.Options.Transaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

	// This migration would not work in Postgres if it were to
	// run in a transaction, so we would want to disable transactions.
	migration = `-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
ALTER TYPE colors ADD VALUE 'yellow' AFTER 'orange';
`

	up, down = parseMigrationContents(migration)

	require.Equal(t, "-- migrate:up transaction:false\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\nALTER TYPE colors ADD VALUE 'yellow' AFTER 'orange';", up.Contents)
	require.Equal(t, false, up.Options.Transaction())

	require.Equal(t, "", down.Contents)
	require.Equal(t, true, down.Options.Transaction())

}
