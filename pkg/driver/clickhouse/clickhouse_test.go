package clickhouse

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func testClickHouseDriver(t *testing.T) *Driver {
	u := dbutil.MustParseURL(os.Getenv("CLICKHOUSE_TEST_URL"))
	drv, err := dbmate.New(u).Driver()
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
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// defaults
		{"clickhouse://myhost", "clickhouse://myhost:9000"},
		// custom port
		{"clickhouse://myhost:1234/mydb", "clickhouse://myhost:1234/mydb"},
		// database parameter
		{"clickhouse://myhost?database=mydb", "clickhouse://myhost:9000/mydb"},
		// username & password
		{"clickhouse://abc:123@myhost/mydb", "clickhouse://abc:123@myhost:9000/mydb"},
		{"clickhouse://abc:@myhost/mydb", "clickhouse://abc@myhost:9000/mydb"},
		// username & password parameter
		{"clickhouse://myhost/mydb?username=abc&password=123", "clickhouse://abc:123@myhost:9000/mydb"},
		{"clickhouse://aaa:111@myhost/mydb?username=bbb&password=222", "clickhouse://bbb:222@myhost:9000/mydb"},
		// custom parameters
		{"clickhouse://myhost/mydb?dial_timeout=200ms", "clickhouse://myhost:9000/mydb?dial_timeout=200ms"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u, err := url.Parse(c.input)
			require.NoError(t, err)

			actual := connectionString(u)
			require.Equal(t, c.expected, actual)
		})
	}
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
	drv.databaseURL.Path = "/fakedb"
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
	require.EqualError(
		t,
		err,
		"code: 516, message: invalid: Authentication failed: password is incorrect or there is no user with such name",
	)
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
		require.EqualError(
			t,
			err,
			"code: 60, message: Table dbmate_test.schema_migrations doesn't exist",
		)

		// use driver function to check the same as above
		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, false, exists)

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
		require.NoError(t, err)

		// use driver function to check the same as above
		exists, err = drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)

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
		require.EqualError(
			t,
			err,
			"code: 60, message: Table dbmate_test.testMigrations doesn't exist",
		)

		// use driver function to check the same as above
		exists, err := drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, false, exists)

		// create table
		err = drv.CreateMigrationsTable(db)
		require.NoError(t, err)

		// migrations table should exist
		err = db.QueryRow("select count(*) from \"testMigrations\"").Scan(&count)
		require.NoError(t, err)

		// use driver function to check the same as above
		exists, err = drv.MigrationsTableExists(db)
		require.NoError(t, err)
		require.Equal(t, true, exists)

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
