// +build go1.16

package dbmate_test

import (
	"github.com/amacneil/dbmate/pkg/dbutil"
	"github.com/stretchr/testify/require"
	"testing"
	"testing/fstest"
)

func TestMemoryFS(t *testing.T) {
	// Test that we can do a migration backed by a MapFS
	// The contents of these files are copied from the testdata
	f := fstest.MapFS{
		"db/migrations/20151129054053_test_migration.sql": &fstest.MapFile{
			Data: []byte(`-- migrate:up
create table users (
  id integer,
  name varchar(255)
);
insert into users (id, name) values (1, 'alice');

-- migrate:down
drop table users;
`),
		},
		"db/migrations/20200227231541_test_posts.sql": &fstest.MapFile{
			Data: []byte(`-- migrate:up
create table posts (
  id integer,
  name varchar(255)
);

-- migrate:down
drop table posts;
`),
		},
	}

	for _, u := range testURLs() {
		t.Run(u.Scheme, func(t *testing.T) {
			db := newTestDB(t, u)
			db.FS = f
			drv, err := db.GetDriver()
			require.NoError(t, err)

			// drop and recreate database
			err = db.Drop()
			require.NoError(t, err)
			err = db.Create()
			require.NoError(t, err)

			// migrate
			err = db.Migrate()
			require.NoError(t, err)

			// verify results
			sqlDB, err := drv.Open()
			require.NoError(t, err)
			defer dbutil.MustClose(sqlDB)

			count := 0
			err = sqlDB.QueryRow(`select count(*) from schema_migrations
				where version = '20151129054053'`).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)

			err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
			require.NoError(t, err)
			require.Equal(t, 1, count)
		})
	}
}
