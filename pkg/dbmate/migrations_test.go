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
	require.Equal(t, false, up.Options.SkipTransaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", down.Contents)
	require.Equal(t, false, down.Options.SkipTransaction())

	migration = `-- migrate:down
drop table users;
-- migrate:up
create table users (id serial, name text);
`

	up, down = parseMigrationContents(migration)

	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);", up.Contents)
	require.Equal(t, false, up.Options.SkipTransaction())

	require.Equal(t, "-- migrate:down\ndrop table users;", down.Contents)
	require.Equal(t, false, down.Options.SkipTransaction())

	migration = `-- migrate:up skip_transaction:true
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
ALTER TYPE colors ADD VALUE 'yellow' AFTER 'orange';
`

	up, down = parseMigrationContents(migration)

	require.Equal(t, "-- migrate:up skip_transaction:true\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\nALTER TYPE colors ADD VALUE 'yellow' AFTER 'orange';", up.Contents)
	require.Equal(t, true, up.Options.SkipTransaction())

	require.Equal(t, "", down.Contents)
	require.Equal(t, false, down.Options.SkipTransaction())

}
