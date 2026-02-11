package dbmate_test

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/mysql"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"

	"github.com/stretchr/testify/require"
	"github.com/zenizh/go-capturer"
)

var rootDir string

func sqliteTestURL(t *testing.T) *url.URL {
	return dbtest.MustParseURL(t, "sqlite:dbmate_test.sqlite3")
}

func sqliteBrokenTestURL(t *testing.T) *url.URL {
	return dbtest.MustParseURL(t, "sqlite:/doesnotexist/dbmate_test.sqlite3")
}

func newTestDB(t *testing.T, u *url.URL) *dbmate.DB {
	var err error

	// find root directory relative to current directory
	if rootDir == "" {
		rootDir, err = filepath.Abs("../..")
		require.NoError(t, err)
	}

	t.Chdir(rootDir + "/testdata")

	db := dbmate.New(u)
	db.AutoDumpSchema = false

	return db
}

func TestNew(t *testing.T) {
	db := dbmate.New(dbtest.MustParseURL(t, "foo:test"))
	require.True(t, db.AutoDumpSchema)
	require.Equal(t, "foo:test", db.DatabaseURL.String())
	require.Equal(t, []string{"./db/migrations"}, db.MigrationsDir)
	require.Equal(t, "schema_migrations", db.MigrationsTableName)
	require.Equal(t, "./db/schema.sql", db.SchemaFile)
	require.False(t, db.WaitBefore)
	require.Equal(t, time.Second, db.WaitInterval)
	require.Equal(t, 60*time.Second, db.WaitTimeout)
	require.Empty(t, db.DriverName)
}

func TestGetDriver(t *testing.T) {
	t.Run("missing URL", func(t *testing.T) {
		db := dbmate.New(nil)
		drv, err := db.Driver()
		require.Nil(t, drv)
		require.EqualError(t, err, "invalid url, have you set your --url flag or DATABASE_URL environment variable?")
	})

	t.Run("missing schema", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "//hi"))
		drv, err := db.Driver()
		require.Nil(t, drv)
		require.EqualError(t, err, "invalid url, have you set your --url flag or DATABASE_URL environment variable?")
	})

	t.Run("invalid driver", func(t *testing.T) {
		db := dbmate.New(dbtest.MustParseURL(t, "foo://bar"))
		drv, err := db.Driver()
		require.EqualError(t, err, "unsupported driver: foo")
		require.Nil(t, drv)
	})

	t.Run("driver name override", func(t *testing.T) {
		// URL has invalid scheme "foo", but we force "sqlite" via DriverName
		db := dbmate.New(dbtest.MustParseURL(t, "foo:dbmate_test.sqlite3"))
		db.DriverName = "sqlite"

		drv, err := db.Driver()
		require.NoError(t, err)
		require.NotNil(t, drv)
		// Verify actual scheme is not changed
		require.Equal(t, db.DatabaseURL.Scheme, "foo")
	})

	t.Run("driver name override with invalid driver", func(t *testing.T) {
		// URL is valid sqlite, but we force a non-existent driver
		db := dbmate.New(dbtest.MustParseURL(t, "sqlite:dbmate_test.sqlite3"))
		db.DriverName = "notadriver"

		drv, err := db.Driver()
		require.EqualError(t, err, "unsupported driver: notadriver")
		require.Nil(t, drv)
	})
}

func TestWait(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))

	// speed up retry loop for testing
	db.WaitInterval = time.Millisecond
	db.WaitTimeout = 5 * time.Millisecond

	t.Run("valid connection", func(t *testing.T) {
		err := db.Wait()
		require.NoError(t, err)
	})

	t.Run("invalid connection", func(t *testing.T) {
		db.DatabaseURL = sqliteBrokenTestURL(t)

		err := db.Wait()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to connect to database: unable to open database file:")
	})
}

func TestDumpSchema(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))

	// create custom schema file directory
	dir := t.TempDir()

	// create schema.sql in subdirectory to test creating directory
	db.SchemaFile = filepath.Join(dir, "/schema/schema.sql")

	// drop database
	err := db.Drop()
	require.NoError(t, err)

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// dump schema
	err = db.DumpSchema()
	require.NoError(t, err)

	// verify schema
	schema, err := os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- Dbmate schema migrations")
}

func TestAutoDumpSchema(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))
	db.AutoDumpSchema = true

	// create custom schema file directory
	dir := t.TempDir()

	// create schema.sql in subdirectory to test creating directory
	db.SchemaFile = filepath.Join(dir, "/schema/schema.sql")

	// drop database
	err := db.Drop()
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)

	// verify schema
	schema, err := os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- Dbmate schema migrations")

	// remove schema
	err = os.Remove(db.SchemaFile)
	require.NoError(t, err)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)

	// schema should be recreated
	schema, err = os.ReadFile(db.SchemaFile)
	require.NoError(t, err)
	require.Contains(t, string(schema), "-- Dbmate schema migrations")
}

func TestLoadSchema(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))
	drv, err := db.Driver()
	require.NoError(t, err)

	// create custom schema file directory
	dir := t.TempDir()

	// create schema.sql in subdirectory to test creating directory
	db.SchemaFile = filepath.Join(dir, "/schema/schema.sql")

	// prepare database state
	err = db.Drop()
	require.NoError(t, err)
	err = db.CreateAndMigrate()
	require.NoError(t, err)

	// schema.sql should not exist
	_, err = os.Stat(db.SchemaFile)
	require.True(t, os.IsNotExist(err))

	// load schema should return error
	err = db.LoadSchema()
	require.Error(t, err)
	require.Regexp(t, "(no such file or directory|system cannot find the path specified)", err.Error())

	// create schema file
	err = db.DumpSchema()
	require.NoError(t, err)

	// schema.sql should exist
	_, err = os.Stat(db.SchemaFile)
	require.NoError(t, err)

	// drop and create database
	err = db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	// load schema.sql into database
	err = db.LoadSchema()
	require.NoError(t, err)

	// verify result
	sqlDB, err := drv.Open()
	require.NoError(t, err)
	defer dbutil.MustClose(sqlDB)

	// check applied migrations
	appliedMigrations, err := drv.SelectMigrations(sqlDB, -1)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"20200227231541": true, "20151129054053": true}, appliedMigrations)

	// users and posts tables have been created
	var count int
	err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
	require.NoError(t, err)
	err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
	require.NoError(t, err)
}

func checkWaitCalled(t *testing.T, db *dbmate.DB, command func() error) {
	oldDatabaseURL := db.DatabaseURL
	db.DatabaseURL = sqliteBrokenTestURL(t)

	err := command()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to connect to database: unable to open database file:")

	db.DatabaseURL = oldDatabaseURL
}

func testWaitBefore(t *testing.T, verbose bool) {
	u := sqliteTestURL(t)
	db := newTestDB(t, u)
	db.Verbose = verbose
	db.WaitBefore = true

	// so that checkWaitCalled returns quickly
	db.WaitInterval = time.Millisecond
	db.WaitTimeout = 5 * time.Millisecond

	// drop database
	err := db.Drop()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.Drop)

	// create
	err = db.Create()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.Create)

	// create and migrate
	err = db.CreateAndMigrate()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.CreateAndMigrate)

	// migrate
	err = db.Migrate()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.Migrate)

	// rollback
	err = db.Rollback()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.Rollback)

	// dump
	err = db.DumpSchema()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.DumpSchema)

	// drop and recreate database before load
	err = db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	// load
	err = db.LoadSchema()
	require.NoError(t, err)
	checkWaitCalled(t, db, db.LoadSchema)
}

func TestWaitBefore(t *testing.T) {
	testWaitBefore(t, false)
}

func TestWaitBeforeVerbose(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		testWaitBefore(t, true)
	})
	re := regexp.MustCompile(`((Applied|Rolled back): .* in) ([\w.,µ]+)`)
	maskedOutput := re.ReplaceAllString(output, "$1 ELAPSED")
	require.Contains(t, maskedOutput,
		`Applying: 20151129054053_test_migration.sql
Last insert ID: 1
Rows affected: 1
Applied: 20151129054053_test_migration.sql in ELAPSED
Applying: 20200227231541_test_posts.sql
Last insert ID: 1
Rows affected: 1
Applied: 20200227231541_test_posts.sql in ELAPSED`)
	require.Contains(t, maskedOutput,
		`Rolling back: 20200227231541_test_posts.sql
Last insert ID: 0
Rows affected: 0
Rolled back: 20200227231541_test_posts.sql in ELAPSED`)
}

func testEachURL(t *testing.T, fn func(*testing.T, *url.URL)) {
	t.Run("sqlite", func(t *testing.T) {
		fn(t, sqliteTestURL(t))
	})

	optionalTestURLs := []string{"MYSQL_TEST_URL", "POSTGRES_TEST_URL"}
	for _, varname := range optionalTestURLs {
		// split on underscore and take first part
		testname := strings.ToLower(strings.Split(varname, "_")[0])
		t.Run(testname, func(t *testing.T) {
			u := dbtest.GetenvURLOrSkip(t, varname)
			fn(t, u)
		})
	}
}

func TestMigrate(t *testing.T) {
	testEachURL(t, func(t *testing.T, u *url.URL) {
		db := newTestDB(t, u)
		drv, err := db.Driver()
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

		// check applied migrations
		appliedMigrations, err := drv.SelectMigrations(sqlDB, -1)
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"20200227231541": true, "20151129054053": true}, appliedMigrations)

		// users table have records
		count := 0
		err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})
}

func TestUp(t *testing.T) {
	testEachURL(t, func(t *testing.T, u *url.URL) {
		db := newTestDB(t, u)
		drv, err := db.Driver()
		require.NoError(t, err)

		// drop database
		err = db.Drop()
		require.NoError(t, err)

		// create and migrate
		err = db.CreateAndMigrate()
		require.NoError(t, err)

		// verify results
		sqlDB, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(sqlDB)

		// check applied migrations
		appliedMigrations, err := drv.SelectMigrations(sqlDB, -1)
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"20200227231541": true, "20151129054053": true}, appliedMigrations)

		// users table have records
		count := 0
		err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})
}

func TestRollback(t *testing.T) {
	testEachURL(t, func(t *testing.T, u *url.URL) {
		db := newTestDB(t, u)
		drv, err := db.Driver()
		require.NoError(t, err)

		// drop and create database
		err = db.Drop()
		require.NoError(t, err)
		err = db.Create()
		require.NoError(t, err)

		// rollback should return error
		err = db.Rollback()
		require.Error(t, err)
		require.ErrorContains(t, err, "can't rollback: no migrations have been applied")

		// migrate database
		err = db.Migrate()
		require.NoError(t, err)

		// verify migration
		sqlDB, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(sqlDB)

		// check applied migrations
		appliedMigrations, err := drv.SelectMigrations(sqlDB, -1)
		require.NoError(t, err)
		require.Equal(t, map[string]bool{"20200227231541": true, "20151129054053": true}, appliedMigrations)

		// users and posts tables have been created
		var count int
		err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
		require.NoError(t, err)
		err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
		require.NoError(t, err)

		// rollback second migration
		err = db.Rollback()
		require.NoError(t, err)

		// one migration remaining
		err = sqlDB.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		// posts table was deleted
		err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
		require.NotNil(t, err)
		require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())

		// users table still exists
		err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
		require.Nil(t, err)

		// rollback first migration
		err = db.Rollback()
		require.NoError(t, err)

		// no migrations remaining
		err = sqlDB.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 0, count)

		// posts table was deleted
		err = sqlDB.QueryRow("select count(*) from posts").Scan(&count)
		require.NotNil(t, err)
		require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())

		// users table was deleted
		err = sqlDB.QueryRow("select count(*) from users").Scan(&count)
		require.NotNil(t, err)
		require.Regexp(t, "(does not exist|doesn't exist|no such table)", err.Error())
	})
}

func TestFindMigrations(t *testing.T) {
	testEachURL(t, func(t *testing.T, u *url.URL) {
		db := newTestDB(t, u)
		drv, err := db.Driver()
		require.NoError(t, err)

		// drop, recreate, and migrate database
		err = db.Drop()
		require.NoError(t, err)
		err = db.Create()
		require.NoError(t, err)

		// verify migration
		sqlDB, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(sqlDB)

		// two pending
		results, err := db.FindMigrations()
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.False(t, results[0].Applied)
		require.False(t, results[1].Applied)
		migrationsTableExists, err := drv.MigrationsTableExists(sqlDB)
		require.NoError(t, err)
		require.False(t, migrationsTableExists)

		// run migrations
		err = db.Migrate()
		require.NoError(t, err)

		// two applied
		results, err = db.FindMigrations()
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.True(t, results[0].Applied)
		require.True(t, results[1].Applied)

		// rollback last migration
		err = db.Rollback()
		require.NoError(t, err)

		// one applied, one pending
		results, err = db.FindMigrations()
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.True(t, results[0].Applied)
		require.False(t, results[1].Applied)
	})
}

func TestFindMigrationsAbsolute(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		db := newTestDB(t, sqliteTestURL(t))
		db.MigrationsDir = []string{"db/migrations"}

		migrations, err := db.FindMigrations()
		require.NoError(t, err)

		require.Equal(t, "db/migrations/20151129054053_test_migration.sql", migrations[0].FilePath)
	})

	t.Run("absolute path", func(t *testing.T) {
		dir := t.TempDir()
		require.True(t, filepath.IsAbs(dir))

		file, err := os.Create(filepath.Join(dir, "1234_example.sql"))
		require.NoError(t, err)
		defer file.Close()

		db := newTestDB(t, sqliteTestURL(t))
		db.MigrationsDir = []string{dir}
		require.Nil(t, db.FS)

		migrations, err := db.FindMigrations()
		require.NoError(t, err)
		require.Len(t, migrations, 1)
		require.Equal(t, dir+"/1234_example.sql", migrations[0].FilePath)
		require.True(t, filepath.IsAbs(migrations[0].FilePath))
		require.Nil(t, migrations[0].FS)
		require.Equal(t, "1234_example.sql", migrations[0].FileName)
		require.Equal(t, "1234", migrations[0].Version)
		require.False(t, migrations[0].Applied)
	})
}

func TestFindMigrationsFS(t *testing.T) {
	mapFS := fstest.MapFS{
		"db/migrations/20151129054053_test_migration.sql": {},
		"db/migrations/001_test_migration.sql": {
			Data: []byte(`-- migrate:up
create table users (id serial, name text);
-- migrate:down
drop table users;
`),
		},
		"db/migrations/002_test_migration.sql":                {},
		"db/migrations/003_not_sql.txt":                       {},
		"db/migrations/missing_version.sql":                   {},
		"db/not_migrations/20151129054053_test_migration.sql": {},
	}

	db := newTestDB(t, sqliteTestURL(t))
	db.FS = mapFS

	// drop and recreate database
	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	actual, err := db.FindMigrations()
	require.NoError(t, err)

	// test migrations are correct and in order
	require.Equal(t, "001_test_migration.sql", actual[0].FileName)
	require.Equal(t, "db/migrations/001_test_migration.sql", actual[0].FilePath)
	require.Equal(t, "001", actual[0].Version)
	require.Equal(t, false, actual[0].Applied)

	require.Equal(t, "002_test_migration.sql", actual[1].FileName)
	require.Equal(t, "db/migrations/002_test_migration.sql", actual[1].FilePath)
	require.Equal(t, "002", actual[1].Version)
	require.Equal(t, false, actual[1].Applied)

	require.Equal(t, "20151129054053_test_migration.sql", actual[2].FileName)
	require.Equal(t, "db/migrations/20151129054053_test_migration.sql", actual[2].FilePath)
	require.Equal(t, "20151129054053", actual[2].Version)
	require.Equal(t, false, actual[2].Applied)

	// test parsing first migration
	parsedSections, err := actual[0].Parse()
	require.Nil(t, err)

	parsed := parsedSections[0]
	require.Equal(t, "-- migrate:up\ncreate table users (id serial, name text);\n", parsed.Up)
	require.True(t, parsed.UpOptions.Transaction())
	require.Equal(t, "-- migrate:down\ndrop table users;\n", parsed.Down)
	require.True(t, parsed.DownOptions.Transaction())
}

func TestFindMigrationsFSMultipleDirs(t *testing.T) {
	mapFS := fstest.MapFS{
		"db/migrations_a/001_test_migration_a.sql": {},
		"db/migrations_a/005_test_migration_a.sql": {},
		"db/migrations_b/003_test_migration_b.sql": {},
		"db/migrations_b/004_test_migration_b.sql": {},
		"db/migrations_c/002_test_migration_c.sql": {},
		"db/migrations_c/006_test_migration_c.sql": {},
	}

	db := newTestDB(t, sqliteTestURL(t))
	db.FS = mapFS
	db.MigrationsDir = []string{"./db/migrations_a", "./db/migrations_b", "./db/migrations_c"}

	// drop and recreate database
	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	actual, err := db.FindMigrations()
	require.NoError(t, err)

	// test migrations are correct and in order
	require.Equal(t, "db/migrations_a/001_test_migration_a.sql", actual[0].FilePath)
	require.Equal(t, "db/migrations_c/002_test_migration_c.sql", actual[1].FilePath)
	require.Equal(t, "db/migrations_b/003_test_migration_b.sql", actual[2].FilePath)
	require.Equal(t, "db/migrations_b/004_test_migration_b.sql", actual[3].FilePath)
	require.Equal(t, "db/migrations_a/005_test_migration_a.sql", actual[4].FilePath)
	require.Equal(t, "db/migrations_c/006_test_migration_c.sql", actual[5].FilePath)
}

func TestMigrateUnrestrictedOrder(t *testing.T) {
	emptyMigration := []byte("-- migrate:up\n-- migrate:down")

	// initialize database
	db := newTestDB(t, sqliteTestURL(t))

	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	// test to apply new migrations on empty database
	db.FS = fstest.MapFS{
		"db/migrations/001_test_migration_a.sql": {Data: emptyMigration},
		"db/migrations/100_test_migration_b.sql": {Data: emptyMigration},
	}

	err = db.Migrate()
	require.NoError(t, err)

	// test to apply an out of order migration
	db.FS = fstest.MapFS{
		"db/migrations/001_test_migration_a.sql": {Data: emptyMigration},
		"db/migrations/100_test_migration_b.sql": {Data: emptyMigration},
		"db/migrations/010_test_migration_c.sql": {Data: emptyMigration},
	}

	err = db.Migrate()
	require.NoError(t, err)
}

func TestMigrateStrictOrder(t *testing.T) {
	emptyMigration := []byte("-- migrate:up\n-- migrate:down")

	// initialize database
	db := newTestDB(t, sqliteTestURL(t))
	db.Strict = true

	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	// test to apply new migrations on empty database
	db.FS = fstest.MapFS{
		"db/migrations/001_test_migration_a.sql": {Data: emptyMigration},
		"db/migrations/010_test_migration_b.sql": {Data: emptyMigration},
	}

	err = db.Migrate()
	require.NoError(t, err)

	// test to apply an in order migration
	db.FS = fstest.MapFS{
		"db/migrations/001_test_migration_a.sql": {Data: emptyMigration},
		"db/migrations/010_test_migration_b.sql": {Data: emptyMigration},
		"db/migrations/100_test_migration_c.sql": {Data: emptyMigration},
	}

	err = db.Migrate()
	require.NoError(t, err)

	// test to apply an out of order migration
	db.FS = fstest.MapFS{
		"db/migrations/001_test_migration_a.sql": {Data: emptyMigration},
		"db/migrations/010_test_migration_b.sql": {Data: emptyMigration},
		"db/migrations/100_test_migration_c.sql": {Data: emptyMigration},
		"db/migrations/050_test_migration_d.sql": {Data: emptyMigration},
	}

	err = db.Migrate()
	require.Error(t, err)
}

func TestMigrateQueryErrorMessage(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))

	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	t.Run("ASCII SQL, error in migrate up", func(t *testing.T) {
		db.FS = fstest.MapFS{
			"db/migrations/001_ascii_error_up.sql": {
				Data: []byte("-- migrate:up\n-- line 2\nnot_valid_sql_ascii_up;\n-- migrate:down"),
			},
		}

		err = db.Migrate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "near \"not_valid_sql_ascii_up\": syntax error")
	})

	t.Run("ASCII SQL, error in migrate down", func(t *testing.T) {
		db.FS = fstest.MapFS{
			"db/migrations/002_ascii_error_down.sql": {
				Data: []byte("-- migrate:up\n--migrate:down\n  not_valid_sql_ascii_down; -- indented"),
			},
		}

		err = db.Migrate()
		require.NoError(t, err)

		err = db.Rollback()
		require.Error(t, err)
		require.Contains(t, err.Error(), "near \"not_valid_sql_ascii_down\": syntax error")
	})

	t.Run("UTF-8 SQL, error in migrate up", func(t *testing.T) {
		db.FS = fstest.MapFS{
			"db/migrations/003_utf8_error_up.sql": {
				Data: []byte("-- migrate:up\n-- line 2\n/* สวัสดี hello */ not_valid_sql_utf8_up;\n--migrate:down"),
			},
		}

		err = db.Migrate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "near \"not_valid_sql_utf8_up\": syntax error")
	})

	t.Run("UTF-8 SQL, error in migrate down", func(t *testing.T) {
		db.FS = fstest.MapFS{
			"db/migrations/004_utf8_error_up.sql": {
				Data: []byte("-- migrate:up\n-- migrate:down\n/* สวัสดี hello */ not_valid_sql_utf8_down;"),
			},
		}

		err = db.Migrate()
		require.NoError(t, err)

		err = db.Rollback()
		require.Error(t, err)
		require.Contains(t, err.Error(), "near \"not_valid_sql_utf8_down\": syntax error")
	})

	t.Run("correctly count with CR-LF line endings present", func(t *testing.T) {
		db.FS = fstest.MapFS{
			"db/migrations/005_cr_lf_line_endings.sql": {
				Data: []byte("-- migrate:up\r\n-- line 2\r\n  not_valid_sql_crlf_up; -- indented\r\n-- migrate:down"),
			},
		}

		err = db.Migrate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "near \"not_valid_sql_crlf_up\": syntax error")
	})
}

func TestMigrationContents(t *testing.T) {
	// ensure Windows CR/LF line endings in migration files work
	testEachURL(t, func(t *testing.T, u *url.URL) {
		db := newTestDB(t, u)
		drv, err := db.Driver()
		require.NoError(t, err)

		err = db.Drop()
		require.NoError(t, err)
		err = db.Create()
		require.NoError(t, err)

		sqlDB, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(sqlDB)

		db.FS = fstest.MapFS{
			"db/migrations/001_win_crlf_migration_empty.sql": {
				Data: []byte("-- migrate:up\r\n-- migrate:down\r\n"),
			},
			"db/migrations/002_win_crlf_migration_basic.sql": {
				Data: []byte("-- migrate:up\r\ncreate table test_win_crlf_basic (\r\n  id integer,\r\n  name varchar(255)\r\n);\r\n-- migrate:down\r\ndrop table test_win_crlf_basic;\r\n"),
			},
			"db/migrations/003_win_crlf_migration_options.sql": {
				Data: []byte("-- migrate:up transaction:true\r\ncreate table test_win_crlf_options (\r\n  id integer,\r\n  name varchar(255)\r\n);\r\n-- migrate:down transaction:true\r\ndrop table test_win_crlf_options;\r\n"),
			},
		}

		// run migrations
		err = db.Migrate()
		require.NoError(t, err)

		// rollback last migration
		err = db.Rollback()
		require.NoError(t, err)
	})
}
