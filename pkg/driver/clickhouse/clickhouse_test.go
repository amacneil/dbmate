package clickhouse

import (
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/amacneil/dbmate/pkg/dbmate"
	"github.com/amacneil/dbmate/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testClickHouseDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(os.Getenv("CLICKHOUSE_TEST_URL"))
	drv, err := dbmate.New(u).GetDriver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestClickHouseDB(t *testing.T) *sql.DB {
	drv := testClickHouseDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// connect database
	db, err := sql.Open("clickhouse", drv.databaseURL.String())
	require.NoError(t, err)

	return db
}

func TestGetDriver(t *testing.T) {
	db := dbmate.New(dbutil.MustParseURL("clickhouse://"))
	drvInterface, err := db.GetDriver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		u, err := url.Parse("clickhouse://user:pass@host/db")
		require.NoError(t, err)

		s := connectionString(u)
		require.Equal(t, "tcp://host:9000?database=db&password=pass&username=user", s)
	})

	t.Run("canonical", func(t *testing.T) {
		u, err := url.Parse("clickhouse://host:9000?database=db&password=pass&username=user")
		require.NoError(t, err)

		s := connectionString(u)
		require.Equal(t, "tcp://host:9000?database=db&password=pass&username=user", s)
	})
}

func TestClickHouseCreateDropDatabase(t *testing.T) {
	drv := testClickHouseDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("clickhouse", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.NoError(t, err)
	}()

	// drop the database
	err = drv.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("clickhouse", drv.databaseURL.String())
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.EqualError(t, err, "code: 81, message: Database dbmate_test doesn't exist")
	}()
}

func TestClickHouseDumpSchema(t *testing.T) {
	drv := testClickHouseDriver(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestClickHouseDB(t)
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	tx, err := db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)
	tx, err = db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc2")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE "+drv.databaseName()+".test_migrations")
	require.Contains(t, string(schema), "--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"INSERT INTO test_migrations (version) VALUES\n"+
		"    ('abc1'),\n"+
		"    ('abc2');\n")

	// DumpSchema should return error if command fails
	values := drv.databaseURL.Query()
	values.Set("database", "fakedb")
	drv.databaseURL.RawQuery = values.Encode()
	db, err = sql.Open("clickhouse", drv.databaseURL.String())
	require.NoError(t, err)

	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.EqualError(t, err, "code: 81, message: Database fakedb doesn't exist")
}

func TestClickHouseDatabaseExists(t *testing.T) {
	drv := testClickHouseDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists()
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestClickHouseDatabaseExists_Error(t *testing.T) {
	drv := testClickHouseDriver(t)
	values := drv.databaseURL.Query()
	values.Set("username", "invalid")
	drv.databaseURL.RawQuery = values.Encode()

	exists, err := drv.DatabaseExists()
	require.EqualError(t, err, "code: 192, message: Unknown user invalid")
	require.Equal(t, false, exists)
}

func TestClickHouseCreateMigrationsTable(t *testing.T) {
	t.Run("default table", func(t *testing.T) {
		drv := testClickHouseDriver(t)
		db := prepTestClickHouseDB(t)
		defer dbutil.MustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.EqualError(t, err, "code: 60, message: Table dbmate_test.schema_migrations doesn't exist.")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})

	t.Run("custom table", func(t *testing.T) {
		drv := testClickHouseDriver(t)
		drv.migrationsTableName = "testMigrations"

		db := prepTestClickHouseDB(t)
		defer dbutil.MustClose(db)

		// migrations table should not exist
		count := 0
		err := db.QueryRow("select count(*) from \"testMigrations\"").Scan(&count)
		require.EqualError(t, err, "code: 60, message: Table dbmate_test.testMigrations doesn't exist.")

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from \"testMigrations\"").Scan(&count)
		require.NoError(t, err)

		// create table should be idempotent
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)
	})
}

func TestClickHouseSelectMigrations(t *testing.T) {
	drv := testClickHouseDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestClickHouseDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into test_migrations (version) values (?)")
	require.NoError(t, err)
	_, err = stmt.Exec("abc2")
	require.NoError(t, err)
	_, err = stmt.Exec("abc1")
	require.NoError(t, err)
	_, err = stmt.Exec("abc3")
	require.NoError(t, err)
	err = tx.Commit()
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

func TestClickHouseInsertMigration(t *testing.T) {
	drv := testClickHouseDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestClickHouseDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	tx, err := db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from test_migrations where version = 'abc1'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestClickHouseDeleteMigration(t *testing.T) {
	drv := testClickHouseDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestClickHouseDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into test_migrations (version) values (?)")
	require.NoError(t, err)
	_, err = stmt.Exec("abc2")
	require.NoError(t, err)
	_, err = stmt.Exec("abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	tx, err = db.Begin()
	require.NoError(t, err)
	err = drv.DeleteMigration(tx, "abc2")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations final where applied").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestClickHousePing(t *testing.T) {
	drv := testClickHouseDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.NoError(t, err)

	// ping invalid host should return error
	drv.databaseURL.Host = "clickhouse:404"
	err = drv.Ping()
	require.Error(t, err)
	require.Contains(t, err.Error(), "connect: connection refused")
}

func TestStatementTimeout(t *testing.T) {
	drv := testClickHouseDriver(t)

	db := prepTestClickHouseDB(t)
	defer dbutil.MustClose(db)

	err := drv.IncreaseStatementTimeout(db, time.Minute)
	require.Error(t, err)
	require.EqualValues(t, dbmate.ErrFeatureNotImplemented, err)
}

func TestClickHouseQuotedMigrationsTableName(t *testing.T) {
	t.Run("default name", func(t *testing.T) {
		drv := testClickHouseDriver(t)
		name := drv.quotedMigrationsTableName()
		require.Equal(t, "schema_migrations", name)
	})

	t.Run("custom name", func(t *testing.T) {
		drv := testClickHouseDriver(t)
		drv.migrationsTableName = "fooMigrations"

		name := drv.quotedMigrationsTableName()
		require.Equal(t, "fooMigrations", name)
	})

	t.Run("quoted name", func(t *testing.T) {
		drv := testClickHouseDriver(t)
		drv.migrationsTableName = "bizarre\"$name"

		name := drv.quotedMigrationsTableName()
		require.Equal(t, `"bizarre""$name"`, name)
	})
}
