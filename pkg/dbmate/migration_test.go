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

	parsedSections, err := migration.Parse()
	require.Nil(t, err)
	parsed := parsedSections[0]
	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
	require.True(t, parsed.UpOptions.Transaction())
	require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
	require.True(t, parsed.DownOptions.Transaction())
}

func TestParseMigrationContents(t *testing.T) {
	t.Run("support the typical use case", func(t *testing.T) {
		migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;`

		parsedSections, err := parseMigrationContents(migration)
		require.Nil(t, err)
		parsed := parsedSections[0]

		require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
		require.Equal(t, true, parsed.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down\ndrop table users;", parsed.Down)
		require.Equal(t, true, parsed.DownOptions.Transaction())
	})

	t.Run("do not require space between '--' and 'migrate'", func(t *testing.T) {
		migration := `
--migrate:up
create table users (id serial, name text);

--migrate:down
drop table users;
`

		parsedSections, err := parseMigrationContents(migration)
		require.Nil(t, err)
		parsed := parsedSections[0]

		require.Equal(t, "--migrate:up\ncreate table users (id serial, name text);\n\n", parsed.Up)
		require.Equal(t, true, parsed.UpOptions.Transaction())

		require.Equal(t, "--migrate:down\ndrop table users;\n", parsed.Down)
		require.Equal(t, true, parsed.DownOptions.Transaction())
	})

	t.Run("require up before down", func(t *testing.T) {
		migration := `-- migrate:down
drop table users;
-- migrate:up
create table users (id serial, name text);
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires '-- migrate:up' to appear before '-- migrate:down'")
	})

	t.Run("support disabling transactions", func(t *testing.T) {
		// e.g., the below would not work in Postgres inside a transaction.
		// It also supports omitting the down block.
		migration := `-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
-- migrate:down transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
-- migrate:up transaction:true
ALTER TYPE colors ADD VALUE 'green' AFTER 'red';
-- migrate:down transaction:true
ALTER TYPE colors ADD VALUE 'green' AFTER 'red';
-- migrate:up
ALTER TYPE colors ADD VALUE 'yellow' AFTER 'red';
-- migrate:down
ALTER TYPE colors ADD VALUE 'yellow' AFTER 'red';
-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'blue' AFTER 'red';
-- migrate:down
ALTER TYPE colors ADD VALUE 'blue' AFTER 'red';
-- migrate:up
ALTER TYPE colors ADD VALUE 'purple' AFTER 'red';
-- migrate:down transaction:false
ALTER TYPE colors ADD VALUE 'purple' AFTER 'red';
`

		parsedSections, err := parseMigrationContents(migration)
		require.Nil(t, err)

		parsedFirstSection := parsedSections[0]

		require.Equal(t, "-- migrate:up transaction:false\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\n", parsedFirstSection.Up)
		require.Equal(t, false, parsedFirstSection.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down transaction:false\nALTER TYPE colors ADD VALUE 'orange' AFTER 'red';\n", parsedFirstSection.Down)
		require.Equal(t, false, parsedFirstSection.DownOptions.Transaction())

		parsedSecondSection := parsedSections[1]

		require.Equal(t, "-- migrate:up transaction:true\nALTER TYPE colors ADD VALUE 'green' AFTER 'red';\n", parsedSecondSection.Up)
		require.Equal(t, true, parsedSecondSection.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down transaction:true\nALTER TYPE colors ADD VALUE 'green' AFTER 'red';\n", parsedSecondSection.Down)
		require.Equal(t, true, parsedSecondSection.DownOptions.Transaction())

		parsedThirdSection := parsedSections[2]

		require.Equal(t, "-- migrate:up\nALTER TYPE colors ADD VALUE 'yellow' AFTER 'red';\n", parsedThirdSection.Up)
		require.Equal(t, true, parsedThirdSection.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down\nALTER TYPE colors ADD VALUE 'yellow' AFTER 'red';\n", parsedThirdSection.Down)
		require.Equal(t, true, parsedThirdSection.DownOptions.Transaction())

		parsedForthSection := parsedSections[3]

		require.Equal(t, "-- migrate:up transaction:false\nALTER TYPE colors ADD VALUE 'blue' AFTER 'red';\n", parsedForthSection.Up)
		require.Equal(t, false, parsedForthSection.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down\nALTER TYPE colors ADD VALUE 'blue' AFTER 'red';\n", parsedForthSection.Down)
		require.Equal(t, true, parsedForthSection.DownOptions.Transaction())

		parsedFifthSection := parsedSections[4]

		require.Equal(t, "-- migrate:up\nALTER TYPE colors ADD VALUE 'purple' AFTER 'red';\n", parsedFifthSection.Up)
		require.Equal(t, true, parsedFifthSection.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down transaction:false\nALTER TYPE colors ADD VALUE 'purple' AFTER 'red';\n", parsedFifthSection.Down)
		require.Equal(t, false, parsedFifthSection.DownOptions.Transaction())
	})

	t.Run("require migrate blocks", func(t *testing.T) {
		migration := `
ALTER TABLE users
ADD COLUMN status status_type DEFAULT 'active';
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires each migration to define an up block with '-- migrate:up'")
	})

	t.Run("require an up block", func(t *testing.T) {
		migration := `-- migrate:down
drop table users;
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires each migration to define an up block with '-- migrate:up'")
	})

	t.Run("require a down block", func(t *testing.T) {
		migration := `-- migrate:up
create table users (id serial, name text);
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires each migration to define a down block with '-- migrate:down'")
	})

	t.Run("allow leading comments and whitespace preceding the migrate blocks", func(t *testing.T) {
		migration := `
-- This migration creates the users table.
-- It'll drop it in the event of a rollback.

-- migrate:up
create table users (id serial, name text);

-- migrate:down
drop table users;
`

		parsedSections, err := parseMigrationContents(migration)
		require.Nil(t, err)
		parsed := parsedSections[0]

		require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n\n", parsed.Up)
		require.Equal(t, true, parsed.UpOptions.Transaction())

		require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
		require.Equal(t, true, parsed.DownOptions.Transaction())
	})

	t.Run("do not allow arbitrary statements preceding the migrate blocks", func(t *testing.T) {
		migration := `
-- create status_type
CREATE TYPE status_type AS ENUM ('active', 'inactive');

-- migrate:up
ALTER TABLE users
ADD COLUMN status status_type DEFAULT 'active';

-- migrate:down
ALTER TABLE users
DROP COLUMN status;
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate does not support statements preceding the '-- migrate:up' block")
	})

	t.Run("ensure Windows CR/LF line endings in migration files are parsed correctly", func(t *testing.T) {
		t.Run("without migration options", func(t *testing.T) {
			migration := "-- migrate:up\r\ncreate table users (id serial, name text);\r\n-- migrate:down\r\ndrop table users;\r\n"

			parsedSections, err := parseMigrationContents(migration)
			require.Nil(t, err)
			parsed := parsedSections[0]

			require.Equal(t, "-- migrate:up\r\ncreate table users (id serial, name text);\r\n", parsed.Up)
			require.Equal(t, migrationOptions{}, parsed.UpOptions)
			require.Equal(t, true, parsed.UpOptions.Transaction())

			require.Equal(t, "-- migrate:down\r\ndrop table users;\r\n", parsed.Down)
			require.Equal(t, migrationOptions{}, parsed.DownOptions)
			require.Equal(t, true, parsed.DownOptions.Transaction())
		})

		t.Run("with migration options", func(t *testing.T) {
			migration := "-- migrate:up transaction:true\r\ncreate table users (id serial, name text);\r\n-- migrate:down transaction:true\r\ndrop table users;\r\n"

			parsedSections, err := parseMigrationContents(migration)
			require.Nil(t, err)
			parsed := parsedSections[0]

			require.Equal(t, "-- migrate:up transaction:true\r\ncreate table users (id serial, name text);\r\n", parsed.Up)
			require.Equal(t, migrationOptions{"transaction": "true"}, parsed.UpOptions)
			require.Equal(t, true, parsed.UpOptions.Transaction())

			require.Equal(t, "-- migrate:down transaction:true\r\ndrop table users;\r\n", parsed.Down)
			require.Equal(t, migrationOptions{"transaction": "true"}, parsed.DownOptions)
			require.Equal(t, true, parsed.DownOptions.Transaction())
		})
	})

	t.Run("require a down block in each section", func(t *testing.T) {
		migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:up
create table statuses (id serial, name text);
-- migrate:down
drop table statuses;
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires each migration to define a down block with '-- migrate:down'")
	})

	t.Run("require an up block in each section", func(t *testing.T) {
		migration := `-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;
-- migrate:down
drop table statuses;
`

		_, err := parseMigrationContents(migration)
		require.Error(t, err, "dbmate requires each migration to define an up block with '-- migrate:up'")
	})
}
