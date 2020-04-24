package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	adminUrl = "oracle://system:passwd@0.0.0.0:1521/ORCLCDB?schema=orauser&passwd=passwd&privileges=create%20table,unlimited%20tablespace"
	appUrl   = "oracle://orauser:passwd@0.0.0.0:1521/ORCLCDB"
)

func oracleAdminTestURL(t *testing.T) *url.URL {
	u, err := url.Parse(adminUrl)
	require.NoError(t, err)

	return u
}
func oracleAppTestURL(t *testing.T) *url.URL {
	u, err := url.Parse(appUrl)
	require.NoError(t, err)

	return u
}

func prepTestOracleDB(t *testing.T) *sql.DB {
	drv := OracleDriver{}
	adminUrl := oracleAdminTestURL(t)
	appUrl := oracleAppTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(adminUrl)

	// create database
	err = drv.CreateDatabase(adminUrl)
	require.NoError(t, err)

	// connect database
	db, err := sql.Open("ora", normalizeOracleURL(appUrl))
	require.NoError(t, err)

	return db
}

func TestOracleCreateDatabase(t *testing.T) {
	db := prepTestOracleDB(t)
	defer mustClose(db)

	err := db.Ping()
	require.NoError(t, err)
}

func TestOracleDropDatabase(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	// drop the database
	u := oracleAdminTestURL(t)
	err := drv.DropDatabase(u)
	require.NoError(t, err)

	err = db.Ping()
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "ORA-01017: invalid username/password; logon denied")
}

func TestOracleDumpSchema(t *testing.T) {
	// TODO
}

func TestOracleDatabaseExists(t *testing.T) {
	drv := OracleDriver{}
	adminUrl := oracleAdminTestURL(t)
	appUrl := oracleAppTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(adminUrl)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(appUrl)
	require.NoError(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(adminUrl)
	require.NoError(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(appUrl)
	require.NoError(t, err)
	require.Equal(t, true, exists)
}

func TestOracleDatabaseExists_Error(t *testing.T) {
	drv := OracleDriver{}
	u := oracleAppTestURL(t)
	u.User = url.User("user-without-password")

	exists, err := drv.DatabaseExists(u)
	require.Contains(t, err.Error(), "ORA-01005: null password given; logon denied")
	require.Equal(t, false, exists)
}

func TestOracleCreateMigrationsTable(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Contains(t, err.Error(), "ORA-00942: table or view does not exist")

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

func TestOracleSelectMigrations(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version) values ('abc2')`)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version) values ('abc1')`)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version) values ('abc3')`)
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

func TestOracleInsertMigration(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestOracleDeleteMigration(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into schema_migrations (version) values ('abc1')`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_migrations (version) values ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestOraclePing(t *testing.T) {
	drv := OracleDriver{}
	db := prepTestOracleDB(t)
	defer mustClose(db)

	// ping admin database
	adminUrl := oracleAdminTestURL(t)
	err := drv.Ping(adminUrl)
	require.NoError(t, err)

	// ping app database
	appUrl := oracleAppTestURL(t)
	err = drv.Ping(appUrl)
	require.NoError(t, err)

	// ping invalid host should return error
	appUrl.Host = "oracle:404"
	err = drv.Ping(appUrl)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ORA-12154: TNS:could not resolve the connect identifier specified")
}
