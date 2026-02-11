package mysql

import (
	"database/sql"
	"fmt"
	"net/url"
	"os/exec"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

type mockExecCmd struct {
	output string
}

func (m *mockExecCmd) Output() ([]byte, error) {
	return []byte(m.output), nil
}

func testMySQLDriver(t *testing.T) *Driver {
	u := dbtest.GetenvURLOrSkip(t, "MYSQL_TEST_URL")
	drv, err := dbmate.New(u).Driver()
	require.NoError(t, err)

	return drv.(*Driver)
}

func prepTestMySQLDB(t *testing.T) *sql.DB {
	drv := testMySQLDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// connect database
	db, err := drv.Open()
	require.NoError(t, err)

	return db
}

func TestGetDriver(t *testing.T) {
	db := dbmate.New(dbtest.MustParseURL(t, "mysql://"))
	drvInterface, err := db.Driver()
	require.NoError(t, err)

	// driver should have URL and default migrations table set
	drv, ok := drvInterface.(*Driver)
	require.True(t, ok)
	require.Equal(t, db.DatabaseURL.String(), drv.databaseURL.String())
	require.Equal(t, "schema_migrations", drv.migrationsTableName)
}

func TestConnectionString(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		u, err := url.Parse("mysql://host/foo")
		require.NoError(t, err)
		require.Equal(t, "", u.Port())

		s := connectionString(u)
		require.Equal(t, "tcp(host:3306)/foo?multiStatements=true", s)
	})

	t.Run("custom", func(t *testing.T) {
		u, err := url.Parse("mysql://bob:secret@host:123/foo?flag=on")
		require.NoError(t, err)
		require.Equal(t, "123", u.Port())

		s := connectionString(u)
		require.Equal(t, "bob:secret@tcp(host:123)/foo?flag=on&multiStatements=true", s)
	})

	t.Run("special chars", func(t *testing.T) {
		u, err := url.Parse("mysql://duhfsd7s:123!@123!@@host:123/foo?flag=on")
		require.NoError(t, err)
		require.Equal(t, "123", u.Port())

		s := connectionString(u)
		require.Equal(t, "duhfsd7s:123!@123!@@tcp(host:123)/foo?flag=on&multiStatements=true", s)
	})

	t.Run("url encoding", func(t *testing.T) {
		u, err := url.Parse("mysql://bob%2Balice:secret%5E%5B%2A%28%29@host:123/foo")
		require.NoError(t, err)
		require.Equal(t, "bob+alice:secret%5E%5B%2A%28%29", u.User.String())
		require.Equal(t, "123", u.Port())

		s := connectionString(u)
		// ensure that '+' is correctly encoded by url.PathUnescape as '+'
		// (not whitespace as url.QueryUnescape generates)
		require.Equal(t, "bob+alice:secret^[*()@tcp(host:123)/foo?multiStatements=true", s)
	})

	t.Run("socket", func(t *testing.T) {
		// test with no user/pass
		u, err := url.Parse("mysql:///foo?socket=/var/run/mysqld/mysqld.sock&flag=on")
		require.NoError(t, err)
		require.Equal(t, "", u.Host)

		s := connectionString(u)
		require.Equal(t, "unix(/var/run/mysqld/mysqld.sock)/foo?flag=on&multiStatements=true", s)

		// test with user/pass
		u, err = url.Parse("mysql://bob:secret@fakehost/foo?socket=/var/run/mysqld/mysqld.sock&flag=on")
		require.NoError(t, err)

		s = connectionString(u)
		require.Equal(t, "bob:secret@unix(/var/run/mysqld/mysqld.sock)/foo?flag=on&multiStatements=true", s)
	})
}

func TestMySQLCreateDropDatabase(t *testing.T) {
	drv := testMySQLDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := drv.Open()
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
		db, err := drv.Open()
		require.NoError(t, err)
		defer dbutil.MustClose(db)

		err = db.Ping()
		require.Error(t, err)
		require.Regexp(t, "Unknown database 'dbmate_test'", err.Error())
	}()
}

func TestMysqldumpVersion(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		version  string
		expected *mysqldumpVersion
	}{
		{"MySQL 5.7.44", "mysqldump", "mysqldump  Ver 10.13 Distrib 5.7.44, for Linux (x86_64)", &mysqldumpVersion{DbType: "mysql", Version: 5.7, Command: "mysqldump"}},
		{"MySQL 8.3.0", "mysqldump", "mysqldump  Ver 8.3.0 for macos13.6 on x86_64 (Homebrew)", &mysqldumpVersion{DbType: "mysql", Version: 8.3, Command: "mysqldump"}},
		{"MySQL 8.4.7", "mysqldump", "mysqldump  Ver 8.4.7 for Linux on x86_64 (MySQL Community Server - GPL)", &mysqldumpVersion{DbType: "mysql", Version: 8.4, Command: "mysqldump"}},
		{"MySQL 9.6.0", "mysqldump", "mysqldump  Ver 9.6.0 for macos26.2 on arm64 (Homebrew)", &mysqldumpVersion{DbType: "mysql", Version: 9.6, Command: "mysqldump"}},
		{"MariaDB 10.11.15", "mysqldump", "mysqldump  Ver 10.19 Distrib 10.11.15-MariaDB, for debian-linux-gnu (x86_64)", &mysqldumpVersion{DbType: "mariadb", Version: 10.11, Command: "mysqldump"}},
		{"MariaDB 11.8.5", "mariadb-dump", "mariadb-dump from 11.8.5-MariaDB, client 10.19 for debian-linux-gnu (x86_64)", &mysqldumpVersion{DbType: "mariadb", Version: 11.8, Command: "mariadb-dump"}},
		{"MariaDB 12.0.2", "mariadb-dump", "mariadb-dump from 12.0.2-MariaDB, client 10.19 for debian-linux-gnu (x86_64)", &mysqldumpVersion{DbType: "mariadb", Version: 12.0, Command: "mariadb-dump"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			origExecCommand := execCommand
			execCommand = func(_ string, _ ...string) execCmd {
				return &mockExecCmd{
					output: c.version,
				}
			}
			origExecLookPath := execLookPath
			execLookPath = func(file string) (string, error) {
				if file == c.command {
					return file, nil
				}
				return file, exec.ErrNotFound
			}
			defer func() {
				execCommand = origExecCommand
				execLookPath = origExecLookPath
			}()

			actual := getMysqldumpVersion()
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestMySQLDumpArgs(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		version  string
		url      string
		expected []string
	}{
		// mysql://bob/mydb
		{"MySQL 5.7.44", "mysqldump", "mysqldump  Ver 10.13 Distrib 5.7.44, for Linux (x86_64)", "mysql://bob/mydb", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl=false", "--host=bob", "mydb"}},
		{"MySQL 8.4.7", "mysqldump", "mysqldump  Ver 8.4.7 for Linux on x86_64 (MySQL Community Server - GPL)", "mysql://bob/mydb", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-mode=DISABLED", "--host=bob", "mydb"}},
		{"MySQL 9.6.0", "mysqldump", "mysqldump  Ver 9.6.0 for macos26.2 on arm64 (Homebrew)", "mysql://bob/mydb", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-mode=DISABLED", "--host=bob", "mydb"}},

		// mysql://alice:pw@bob:5678/mydb
		{"MySQL 5.7.44", "mysqldump", "mysqldump  Ver 10.13 Distrib 5.7.44, for Linux (x86_64)", "mysql://alice:pw@bob:5678/mydb", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl=false", "--host=bob", "--port=5678", "--user=alice", "--password=pw", "mydb"}},
		{"MySQL 8.4.7", "mysqldump", "mysqldump  Ver 8.4.7 for Linux on x86_64 (MySQL Community Server - GPL)", "mysql://alice:pw@bob:5678/mydb", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-mode=DISABLED", "--host=bob", "--port=5678", "--user=alice", "--password=pw", "mydb"}},

		// mysql://alice:pw@bob:5678/mydb?tls=skip-verify
		{"MySQL 5.7.44", "mysqldump", "mysqldump  Ver 10.13 Distrib 5.7.44, for Linux (x86_64)", "mysql://alice:pw@bob:5678/mydb?tls=skip-verify", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-verify-server-cert=false", "--host=bob", "--port=5678", "--user=alice", "--password=pw", "mydb"}},
		{"MySQL 8.4.7", "mysqldump", "mysqldump  Ver 8.4.7 for Linux on x86_64 (MySQL Community Server - GPL)", "mysql://alice:pw@bob:5678/mydb?tls=skip-verify", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-mode=PREFERRED", "--host=bob", "--port=5678", "--user=alice", "--password=pw", "mydb"}},

		// mysql://alice:pw@bob:5678/mydb?socket=/var/run/mysqld/mysqld.sock
		{"MySQL 5.7.44", "mysqldump", "mysqldump  Ver 10.13 Distrib 5.7.44, for Linux (x86_64)", "mysql://alice:pw@bob:5678/mydb?socket=/var/run/mysqld/mysqld.sock", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl=false", "--socket=/var/run/mysqld/mysqld.sock", "--user=alice", "--password=pw", "mydb"}},
		{"MySQL 8.4.7", "mysqldump", "mysqldump  Ver 8.4.7 for Linux on x86_64 (MySQL Community Server - GPL)", "mysql://alice:pw@bob:5678/mydb?socket=/var/run/mysqld/mysqld.sock", []string{"--opt", "--routines", "--no-data", "--skip-dump-date", "--skip-add-drop-table", "--ssl-mode=DISABLED", "--socket=/var/run/mysqld/mysqld.sock", "--user=alice", "--password=pw", "mydb"}},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s__%s", c.name, c.url), func(t *testing.T) {
			origExecCommand := execCommand
			execCommand = func(_ string, _ ...string) execCmd {
				return &mockExecCmd{
					output: c.version,
				}
			}
			origExecLookPath := execLookPath
			execLookPath = func(file string) (string, error) {
				if file == c.command {
					return file, nil
				}
				return file, exec.ErrNotFound
			}
			defer func() {
				execCommand = origExecCommand
				execLookPath = origExecLookPath
			}()

			ver := getMysqldumpVersion()

			drv := testMySQLDriver(t)
			drv.databaseURL = dbtest.MustParseURL(t, c.url)

			actual := drv.mysqldumpArgs(ver)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestMySQLDumpSchema(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE `test_migrations`")
	require.Contains(t, string(schema), "\n-- Dump completed\n\n"+
		"--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"LOCK TABLES `test_migrations` WRITE;\n"+
		"INSERT INTO `test_migrations` (version) VALUES\n"+
		"  ('abc1'),\n"+
		"  ('abc2');\n"+
		"UNLOCK TABLES;\n")

	// DumpSchema should return error if command fails
	drv.databaseURL.Path = "/fakedb"
	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown database 'fakedb'")
}

func TestMySQLDumpSchemaContainsNoAutoIncrement(t *testing.T) {
	drv := testMySQLDriver(t)

	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// create table with AUTO_INCREMENT column
	_, err = db.Exec(`create table foo_table (id int not null primary key auto_increment)`)
	require.NoError(t, err)

	// create a record
	_, err = db.Exec(`insert into foo_table values ()`)
	require.NoError(t, err)

	// AUTO_INCREMENT should appear on the table definition
	var tblName, tblCreate string
	err = db.QueryRow(`show create table foo_table`).Scan(&tblName, &tblCreate)
	require.NoError(t, err)
	require.Contains(t, tblCreate, "AUTO_INCREMENT=")

	// AUTO_INCREMENT should not appear in the dump
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.NotContains(t, string(schema), "AUTO_INCREMENT=")
}

func TestMySQLDatabaseExists(t *testing.T) {
	drv := testMySQLDriver(t)

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

func TestMySQLDatabaseExists_Error(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.databaseURL.User = url.User("invalid")

	exists, err := drv.DatabaseExists()
	require.Error(t, err)
	require.Regexp(t, "Access denied for user 'invalid'@", err.Error())
	require.Equal(t, false, exists)
}

func TestMySQLCreateMigrationsTable(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.Error(t, err)
	require.Regexp(t, "Table 'dbmate_test.test_migrations' doesn't exist", err.Error())

	// create table
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.NoError(t, err)
}

func TestMySQLSelectMigrations(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
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

func TestMySQLInsertMigration(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.NoError(t, err)

	err = db.QueryRow("select count(*) from test_migrations where version = 'abc1'").
		Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLDeleteMigration(t *testing.T) {
	drv := testMySQLDriver(t)
	drv.migrationsTableName = "test_migrations"

	db := prepTestMySQLDB(t)
	defer dbutil.MustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	_, err = db.Exec(`insert into test_migrations (version)
		values ('abc1'), ('abc2')`)
	require.NoError(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.NoError(t, err)

	count := 0
	err = db.QueryRow("select count(*) from test_migrations").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestMySQLPing(t *testing.T) {
	drv := testMySQLDriver(t)

	// drop any existing database
	err := drv.DropDatabase()
	require.NoError(t, err)

	// ping database
	err = drv.Ping()
	require.NoError(t, err)

	// ping invalid host should return error
	drv.databaseURL.Host = "mysql:404"
	err = drv.Ping()
	require.Error(t, err)
	require.Contains(t, err.Error(), "connect: connection refused")
}

func TestMySQLQuotedMigrationsTableName(t *testing.T) {
	t.Run("default name", func(t *testing.T) {
		drv := testMySQLDriver(t)
		name := drv.quotedMigrationsTableName()
		require.Equal(t, "`schema_migrations`", name)
	})

	t.Run("custom name", func(t *testing.T) {
		drv := testMySQLDriver(t)
		drv.migrationsTableName = "fooMigrations"

		name := drv.quotedMigrationsTableName()
		require.Equal(t, "`fooMigrations`", name)
	})
}
