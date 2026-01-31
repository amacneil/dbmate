package clickhouse

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/stretchr/testify/require"
)

func assertDatabaseExists(t *testing.T, drv *Driver, shouldExist bool) {
	db, err := sql.Open("clickhouse", drv.databaseURL.String())
	require.NoError(t, err)
	defer dbutil.MustClose(db)

	err = db.Ping()
	if shouldExist {
		require.NoError(t, err)
	} else {
		require.EqualError(t, err, "code: 81, message: Database dbmate_test doesn't exist")
	}
}

// Makes sure driver creatinon is atomic
func TestDriverCreationSanity(t *testing.T) {
	u := dbtest.GetenvURLOrSkip(t, "CLICKHOUSE_CLUSTER_01_TEST_URL")
	u.RawQuery = "on_cluster"
	dbm := dbmate.New(u)

	drv, err := dbm.Driver()
	require.NoError(t, err)

	drvAgain, err := dbm.Driver()
	require.NoError(t, err)

	require.Equal(t, drv, drvAgain)
}

func TestOnClusterClause(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// not on cluster
		{"clickhouse://myhost:9000", ""},
		// on_cluster supplied
		{"clickhouse://myhost:9000?on_cluster", " ON CLUSTER '{cluster}'"},
		// on_cluster with supplied macro
		{"clickhouse://myhost:9000?on_cluster&cluster_macro={cluster2}", " ON CLUSTER '{cluster2}'"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			drv := testClickHouseDriverURL(t, dbtest.MustParseURL(t, c.input))
			actual := drv.onClusterClause()
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestClickHouseCreateDropDatabaseOnCluster(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)

	// drop any existing database
	err := drv01.DropDatabase()
	require.NoError(t, err)

	// create database
	err = drv01.CreateDatabase()
	require.NoError(t, err)

	// check that database exists and we can connect to it
	assertDatabaseExists(t, drv01, true)
	// check that database exists on the other clickhouse node and we can connect to it
	assertDatabaseExists(t, drv02, true)

	// drop the database
	err = drv01.DropDatabase()
	require.NoError(t, err)

	// check that database no longer exists
	assertDatabaseExists(t, drv01, false)
	// check that database no longer exists on the other clickhouse node
	assertDatabaseExists(t, drv02, false)
}

func TestClickHouseDumpSchemaOnCluster(t *testing.T) {
	drv := testClickHouseDriverCluster01(t)
	drv.migrationsTableName = "test_migrations"

	// prepare database
	db := prepTestClickHouseDB(t, drv)
	defer dbutil.MustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.NoError(t, err)

	// insert migration
	tx, err := db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc1", "checksum1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)
	tx, err = db.Begin()
	require.NoError(t, err)
	err = drv.InsertMigration(tx, "abc2", "checksum2")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(db)
	require.NoError(t, err)
	require.Contains(t, string(schema), "CREATE TABLE "+drv.databaseName()+".test_migrations")
	require.Contains(t, string(schema), "ENGINE = ReplicatedReplacingMergeTree")
	require.Contains(t, string(schema), "--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"INSERT INTO test_migrations (version, checksum) VALUES\n"+
		"    ('abc1', 'checksum1'),\n"+
		"    ('abc2', 'checksum2');\n")

	// DumpSchema should return error if command fails
	drv.databaseURL.Path = "/fakedb"
	db, err = sql.Open("clickhouse", drv.databaseURL.String())
	require.NoError(t, err)

	schema, err = drv.DumpSchema(db)
	require.Nil(t, schema)
	require.EqualError(t, err, "code: 81, message: Database fakedb doesn't exist")
}

func TestClickHouseCreateMigrationsTableOnCluster(t *testing.T) {
	testCases := []struct {
		name              string
		migrationsTable   string
		expectedTableName string
	}{
		{
			name:              "default table",
			migrationsTable:   "",
			expectedTableName: "schema_migrations",
		},
		{
			name:              "custom table",
			migrationsTable:   "testMigrations",
			expectedTableName: "\"testMigrations\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drv01 := testClickHouseDriverCluster01(t)
			drv02 := testClickHouseDriverCluster02(t)
			if tc.migrationsTable != "" {
				drv01.migrationsTableName = tc.migrationsTable
				drv02.migrationsTableName = tc.migrationsTable
			}

			db01 := prepTestClickHouseDB(t, drv01)
			defer dbutil.MustClose(db01)

			db02 := prepTestClickHouseDB(t, drv02)
			defer dbutil.MustClose(db02)

			// migrations table should not exist
			exists, err := drv01.MigrationsTableExists(db01)
			require.NoError(t, err)
			require.Equal(t, false, exists)

			// migrations table should not exist on the other node
			exists, err = drv02.MigrationsTableExists(db02)
			require.NoError(t, err)
			require.Equal(t, false, exists)

			// create table
			err = drv01.CreateMigrationsTable(db01)
			require.NoError(t, err)

			// migrations table should exist
			exists, err = drv01.MigrationsTableExists(db01)
			require.NoError(t, err)
			require.Equal(t, true, exists)

			// migrations table should exist on other node
			exists, err = drv02.MigrationsTableExists(db02)
			require.NoError(t, err)
			require.Equal(t, true, exists)

			// create table should be idempotent
			err = drv01.CreateMigrationsTable(db01)
			require.NoError(t, err)
		})
	}
}

func TestClickHouseSelectMigrationsOnCluster(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)
	drv01.migrationsTableName = "test_migrations"
	drv02.migrationsTableName = "test_migrations"

	db01 := prepTestClickHouseDB(t, drv01)
	defer dbutil.MustClose(db01)

	db02 := prepTestClickHouseDB(t, drv02)
	defer dbutil.MustClose(db02)

	err := drv01.CreateMigrationsTable(db01)
	require.NoError(t, err)

	tx, err := db01.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into test_migrations (version, checksum) values (?, ?)")
	require.NoError(t, err)
	_, err = stmt.Exec("abc2", nil)
	require.NoError(t, err)
	_, err = stmt.Exec("abc1", "checksum1")
	require.NoError(t, err)
	_, err = stmt.Exec("abc3", "checksum3")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	migrations01, err := drv01.SelectMigrations(db01, -1)
	require.NoError(t, err)
	require.Equal(t, "checksum1", *migrations01["abc1"])
	require.Equal(t, "", *migrations01["abc2"])
	require.Equal(t, "checksum3", *migrations01["abc3"])

	// Assert select on other node
	migrations02, err := drv02.SelectMigrations(db02, -1)
	require.NoError(t, err)
	require.Equal(t, "checksum1", *migrations02["abc1"])
	require.Equal(t, "", *migrations02["abc2"])
	require.Equal(t, "checksum3", *migrations02["abc3"])

	// test limit param
	migrations01, err = drv01.SelectMigrations(db01, 1)
	require.NoError(t, err)
	require.Equal(t, "checksum3", *migrations01["abc3"])
	require.Equal(t, (*string)(nil), migrations01["abc2"])
	require.Equal(t, (*string)(nil), migrations01["abc1"])

	// test limit param on other node
	migrations02, err = drv02.SelectMigrations(db02, 1)
	require.NoError(t, err)
	require.Equal(t, "checksum3", *migrations02["abc3"])
	require.Equal(t, (*string)(nil), migrations02["abc2"])
	require.Equal(t, (*string)(nil), migrations02["abc1"])
}

func TestClickHouseInsertMigrationOnCluster(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)
	drv01.migrationsTableName = "test_migrations"
	drv02.migrationsTableName = "test_migrations"

	db01 := prepTestClickHouseDB(t, drv01)
	defer dbutil.MustClose(db01)

	db02 := prepTestClickHouseDB(t, drv02)
	defer dbutil.MustClose(db02)

	err := drv01.CreateMigrationsTable(db01)
	require.NoError(t, err)

	count01 := 0
	err = db01.QueryRow("select count(*) from test_migrations").Scan(&count01)
	require.NoError(t, err)
	require.Equal(t, 0, count01)

	count02 := 0
	err = db02.QueryRow("select count(*) from test_migrations").Scan(&count02)
	require.NoError(t, err)
	require.Equal(t, 0, count02)

	// insert migration
	tx, err := db01.Begin()
	require.NoError(t, err)
	err = drv01.InsertMigration(tx, "abc1", "checksum1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	err = db01.QueryRow("select count(*) from test_migrations where version = 'abc1'").Scan(&count01)
	require.NoError(t, err)
	require.Equal(t, 1, count01)

	err = db02.QueryRow("select count(*) from test_migrations where version = 'abc1'").Scan(&count02)
	require.NoError(t, err)
	require.Equal(t, 1, count02)
}

func TestClickHouseDeleteMigrationOnCluster(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)
	drv01.migrationsTableName = "test_migrations"
	drv02.migrationsTableName = "test_migrations"

	db01 := prepTestClickHouseDB(t, drv01)
	defer dbutil.MustClose(db01)

	db02 := prepTestClickHouseDB(t, drv02)
	defer dbutil.MustClose(db02)

	err := drv01.CreateMigrationsTable(db01)
	require.NoError(t, err)

	tx, err := db01.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("insert into test_migrations (version) values (?)")
	require.NoError(t, err)
	_, err = stmt.Exec("abc2")
	require.NoError(t, err)
	_, err = stmt.Exec("abc1")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	tx, err = db01.Begin()
	require.NoError(t, err)
	err = drv01.DeleteMigration(tx, "abc2")
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	count01 := 0
	err = db01.QueryRow("select count(*) from test_migrations final where applied").Scan(&count01)
	require.NoError(t, err)
	require.Equal(t, 1, count01)

	count02 := 0
	err = db02.QueryRow("select count(*) from test_migrations final where applied").Scan(&count02)
	require.NoError(t, err)
	require.Equal(t, 1, count02)
}

func TestClickHouseAddChecksumColumnOnClusterNonReplicated(t *testing.T) {
	drv01 := testClickHouseDriverCluster01(t)
	drv02 := testClickHouseDriverCluster02(t)
	// Use a distinct table name to avoid collisions
	tableName := "test_migrations_nonrepl"
	drv01.migrationsTableName = tableName
	drv02.migrationsTableName = tableName

	db01 := prepTestClickHouseDB(t, drv01)
	defer dbutil.MustClose(db01)

	db02 := prepTestClickHouseDB(t, drv02)
	defer dbutil.MustClose(db02)

	// Create migrations table WITHOUT checksum column, using non-replicated engine
	// Even though OnCluster is true, we use ReplacingMergeTree (non-replicated)
	// to test that ON CLUSTER clause is needed for DDL propagation across nodes.
	engineClause := "ReplacingMergeTree(ts)"
	createTableSQL := fmt.Sprintf(`
		create table if not exists %s%s (
			version String,
			ts DateTime default now(),
			applied UInt8 default 1
		) engine = %s
		primary key version
		order by version
	`, drv01.quotedMigrationsTableName(), drv01.onClusterClause(), engineClause)
	_, err := db01.Exec(createTableSQL)
	require.NoError(t, err)

	// verify table exists on both nodes (because of ON CLUSTER clause)
	exists, err := drv01.MigrationsTableExists(db01)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = drv02.MigrationsTableExists(db02)
	require.NoError(t, err)
	require.True(t, exists)

	// verify checksum column does not exist initially
	hasChecksum, err := drv01.HasChecksumColumn(db01)
	require.NoError(t, err)
	require.False(t, hasChecksum)
	hasChecksum, err = drv02.HasChecksumColumn(db02)
	require.NoError(t, err)
	require.False(t, hasChecksum)

	// add checksum column
	err = drv01.AddChecksumColumn(db01)
	require.NoError(t, err)

	// verify checksum column exists on node1 (where ALTER TABLE executed)
	hasChecksum, err = drv01.HasChecksumColumn(db01)
	require.NoError(t, err)
	require.True(t, hasChecksum, "checksum column should exist on node1")

	// verify checksum column exists on node2 (because of ON CLUSTER clause)
	hasChecksum, err = drv02.HasChecksumColumn(db02)
	require.NoError(t, err)
	require.True(t, hasChecksum, "checksum column should exist on node2")
}
