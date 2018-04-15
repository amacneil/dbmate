package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func mySQLTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("mysql://root:root@mysql/dbmate")
	require.Nil(t, err)

	return u
}

func prepTestMySQLDB(t *testing.T) *sql.DB {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// connect database
	db, err := drv.Open(u)
	require.Nil(t, err)

	return db
}

func TestNormalizeMySQLURLDefaults(t *testing.T) {
	u, err := url.Parse("mysql://host/foo")
	require.Nil(t, err)
	require.Equal(t, "", u.Port())

	s := normalizeMySQLURL(u)
	require.Equal(t, "tcp(host:3306)/foo?multiStatements=true", s)
}

func TestNormalizeMySQLURLCustom(t *testing.T) {
	u, err := url.Parse("mysql://bob:secret@host:123/foo?flag=on")
	require.Nil(t, err)
	require.Equal(t, "123", u.Port())

	s := normalizeMySQLURL(u)
	require.Equal(t, "bob:secret@tcp(host:123)/foo?flag=on&multiStatements=true", s)
}

func TestMySQLCreateDropDatabase(t *testing.T) {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := drv.Open(u)
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.Nil(t, err)
	}()

	// drop the database
	err = drv.DropDatabase(u)
	require.Nil(t, err)

	// check that database no longer exists
	func() {
		db, err := drv.Open(u)
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.NotNil(t, err)
		require.Regexp(t, "Unknown database 'dbmate'", err.Error())
	}()
}

func TestMySQLDumpSchema(t *testing.T) {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)

	// prepare database
	db := prepTestMySQLDB(t)
	defer mustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.Nil(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.Nil(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(u, db)
	require.Nil(t, err)
	require.Contains(t, string(schema), "CREATE TABLE `schema_migrations`")
	require.Contains(t, string(schema), "\n-- Dump completed\n\n"+
		"--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"LOCK TABLES `schema_migrations` WRITE;\n"+
		"INSERT INTO `schema_migrations` (version) VALUES\n"+
		"  ('abc1'),\n"+
		"  ('abc2');\n"+
		"UNLOCK TABLES;\n")

	// DumpSchema should return error if command fails
	u.Path = "/fakedb"
	schema, err = drv.DumpSchema(u, db)
	require.Nil(t, schema)
	require.NotNil(t, err)
	require.Equal(t, "mysqldump: Got error: 1049: \"Unknown database 'fakedb'\" "+
		"when selecting the database", err.Error())
}

func TestMySQLDatabaseExists(t *testing.T) {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, true, exists)
}

func TestMySQLDatabaseExists_Error(t *testing.T) {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)
	u.User = url.User("invalid")

	exists, err := drv.DatabaseExists(u)
	require.Regexp(t, "Access denied for user 'invalid'@", err.Error())
	require.Equal(t, false, exists)
}

func TestMySQLCreateMigrationsTable(t *testing.T) {
	drv := MySQLDriver{}
	db := prepTestMySQLDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Regexp(t, "Table 'dbmate.schema_migrations' doesn't exist", err.Error())

	// create table
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)
}

func TestMySQLSelectMigrations(t *testing.T) {
	drv := MySQLDriver{}
	db := prepTestMySQLDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.Nil(t, err)

	migrations, err := drv.SelectMigrations(db, -1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
}

func TestMySQLInsertMigration(t *testing.T) {
	drv := MySQLDriver{}
	db := prepTestMySQLDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.Nil(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").
		Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLDeleteMigration(t *testing.T) {
	drv := MySQLDriver{}
	db := prepTestMySQLDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc1'), ('abc2')`)
	require.Nil(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLPing(t *testing.T) {
	drv := MySQLDriver{}
	u := mySQLTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// ping database
	err = drv.Ping(u)
	require.Nil(t, err)

	// ping invalid host should return error
	u.Host = "mysql:404"
	err = drv.Ping(u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "getsockopt: connection refused")
}
