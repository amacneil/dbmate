package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func clickhouseTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("clickhouse://clickhouse:9000?database=dbmate")
	require.NoError(t, err)

	return u
}

func prepTestClickHouseDB(t *testing.T) *sql.DB {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// connect database
	db, err := sql.Open("clickhouse", u.String())
	require.NoError(t, err)

	return db
}

func TestNormalizeClickHouseURLSimplified(t *testing.T) {
	u, err := url.Parse("clickhouse://user:pass@host/db")
	require.NoError(t, err)

	s := normalizeClickHouseURL(u).String()
	require.Equal(t, "tcp://host:9000?database=db&password=pass&username=user", s)
}

func TestNormalizeClickHouseURLCanonical(t *testing.T) {
	u, err := url.Parse("clickhouse://host:9000?database=db&password=pass&username=user")
	require.NoError(t, err)

	s := normalizeClickHouseURL(u).String()
	require.Equal(t, "tcp://host:9000?database=db&password=pass&username=user", s)
}

func TestClickHouseCreateDropDatabase(t *testing.T) {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("clickhouse", u.String())
		require.NoError(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.NoError(t, err)
	}()

	// drop the database
	err = drv.DropDatabase(u)
	require.NoError(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("clickhouse", u.String())
		require.NoError(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.EqualError(t, err, "code: 81, message: Database dbmate doesn't exist")
	}()
}

func TestClickHouseDumpSchema(t *testing.T) {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)

	// prepare database
	db := prepTestClickHouseDB(t)
	defer mustClose(db)
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
	schema, err := drv.DumpSchema(u, db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE "+drv.databaseName(u)+".schema_migrations")
	require.Contains(t, string(schema), "--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"INSERT INTO schema_migrations (version) VALUES\n"+
		"    ('abc1'),\n"+
		"    ('abc2');\n")

	// DumpSchema should return error if command fails
	values := u.Query()
	values.Set("database", "fakedb")
	u.RawQuery = values.Encode()
	db, err = sql.Open("clickhouse", u.String())
	require.NoError(t, err)

	schema, err = drv.DumpSchema(u, db)
	require.Nil(t, schema)
	require.EqualError(t, err, "code: 81, message: Database fakedb doesn't exist")
}

func TestClickHouseDatabaseExists(t *testing.T) {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestClickHouseDatabaseExists_Error(t *testing.T) {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)
	values := u.Query()
	values.Set("username", "invalid")
	u.RawQuery = values.Encode()

	exists, err := drv.DatabaseExists(u)
	require.EqualError(t, err, "code: 192, message: Unknown user invalid")
	require.Equal(t, false, exists)
}

func TestClickHouseCreateMigrationsTable(t *testing.T) {
	drv := ClickHouseDriver{}
	db := prepTestClickHouseDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.EqualError(t, err, "code: 60, message: Table dbmate.schema_migrations doesn't exist.")

	// create table
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)
}

func TestClickHouseSelectMigrations(t *testing.T) {
	drv := ClickHouseDriver{}
	db := prepTestClickHouseDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into schema_migrations (version) values (?)")
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
	drv := ClickHouseDriver{}
	db := prepTestClickHouseDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	tx, err := db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestClickHouseDeleteMigration(t *testing.T) {
	drv := ClickHouseDriver{}
	db := prepTestClickHouseDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into schema_migrations (version) values (?)")
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
	err = db.QueryRow("select count(*) from schema_migrations final where applied").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestClickHousePing(t *testing.T) {
	drv := ClickHouseDriver{}
	u := clickhouseTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	// ping database
	err = drv.Ping(u)
	require.NoError(t, err)

	// ping invalid host should return error
	u.Host = "clickhouse:404"
	err = drv.Ping(u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "connect: connection refused")
}
