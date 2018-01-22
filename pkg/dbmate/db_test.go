package dbmate

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var testdataDir string

func newTestDB(t *testing.T, u *url.URL) *DB {
	var err error

	// only chdir once, because testdata is relative to current directory
	if testdataDir == "" {
		testdataDir, err = filepath.Abs("../../testdata")
		require.Nil(t, err)

		err = os.Chdir(testdataDir)
		require.Nil(t, err)
	}

	return New(u)
}

func testURLs(t *testing.T) []*url.URL {
	return []*url.URL{
		postgresTestURL(t),
		mySQLTestURL(t),
		sqliteTestURL(t),
	}
}

func testMigrateURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop and recreate database
	err := db.Drop()
	require.Nil(t, err)
	err = db.Create()
	require.Nil(t, err)

	// migrate
	err = db.Migrate()
	require.Nil(t, err)

	// verify results
	sqlDB, err := GetDriverOpen(u)
	require.Nil(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestMigrate(t *testing.T) {
	for _, u := range testURLs(t) {
		testMigrateURL(t, u)
	}
}

func testUpURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop database
	err := db.Drop()
	require.Nil(t, err)

	// create and migrate
	err = db.CreateAndMigrate()
	require.Nil(t, err)

	// verify results
	sqlDB, err := GetDriverOpen(u)
	require.Nil(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestUp(t *testing.T) {
	for _, u := range testURLs(t) {
		testUpURL(t, u)
	}
}

func testRollbackURL(t *testing.T, u *url.URL) {
	db := newTestDB(t, u)

	// drop, recreate, and migrate database
	err := db.Drop()
	require.Nil(t, err)
	err = db.Create()
	require.Nil(t, err)
	err = db.Migrate()
	require.Nil(t, err)

	// verify migration
	sqlDB, err := GetDriverOpen(u)
	require.Nil(t, err)
	defer mustClose(sqlDB)

	count := 0
	err = sqlDB.QueryRow(`select count(*) from schema_migrations
		where version = '20151129054053'`).Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)

	// rollback
	err = db.Rollback()
	require.Nil(t, err)

	// verify rollback
	err = sqlDB.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.NotNil(t, err)
	require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())
}

func TestRollback(t *testing.T) {
	for _, u := range testURLs(t) {
		testRollbackURL(t, u)
	}
}
