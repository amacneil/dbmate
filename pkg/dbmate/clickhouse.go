package dbmate

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/ClickHouse/clickhouse-go"
)

func init() {
	RegisterDriver(ClickHouseDriver{}, "clickhouse")
}

// ClickHouseDriver provides top level database functions
type ClickHouseDriver struct {
}

// Open creates a new database connection
func (drv ClickHouseDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("clickhouse", u.String())
}

func (drv ClickHouseDriver) openClickHouseDB(u *url.URL) (*sql.DB, error) {
	// connect to clickhouse database
	clickhouseURL := *u
	values, _ := url.ParseQuery(clickhouseURL.RawQuery)
	values.Set("database", "default")
	clickhouseURL.RawQuery = values.Encode()

	return drv.Open(&clickhouseURL)
}

func (drv ClickHouseDriver) databaseName(u *url.URL) string {
	values, _ := url.ParseQuery(u.RawQuery)
	name := values.Get("database")
	if name == "" {
		name = "default"
	}
	return name
}

var clickhouseValidIdentifier = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]*$`)

func clickhouseQuoteIdentifier(str string) string {
	if clickhouseValidIdentifier.MatchString(str) {
		return str
	}

	str = strings.Replace(str, `"`, `""`, -1)

	return fmt.Sprintf(`"%s"`, str)
}

// CreateDatabase creates the specified database
func (drv ClickHouseDriver) CreateDatabase(u *url.URL) error {
	name := drv.databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openClickHouseDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec("create database " + clickhouseQuoteIdentifier(name))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv ClickHouseDriver) DropDatabase(u *url.URL) error {
	name := drv.databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openClickHouseDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec("drop database if exists " + clickhouseQuoteIdentifier(name))

	return err
}

func clickhouseSchemaDump(db *sql.DB, buf *bytes.Buffer, databaseName string) error {
	buf.WriteString("\n--\n-- Database schema\n--\n\n")

	buf.WriteString("CREATE DATABASE " + clickhouseQuoteIdentifier(databaseName) + " IF NOT EXISTS;\n\n")

	tables, err := queryColumn(db, "show tables")
	if err != nil {
		return err
	}
	sort.Strings(tables)

	for _, table := range tables {
		var clause string
		err = db.QueryRow("show create table " + clickhouseQuoteIdentifier(table)).Scan(&clause)
		if err != nil {
			return err
		}
		buf.WriteString(clause + ";\n\n")
	}
	return nil
}

func clickhouseSchemaMigrationsDump(db *sql.DB, buf *bytes.Buffer) error {
	// load applied migrations
	migrations, err := queryColumn(db,
		"select version from schema_migrations final where applied order by version asc",
	)
	if err != nil {
		return err
	}

	quoter := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	for i := range migrations {
		migrations[i] = "'" + quoter.Replace(migrations[i]) + "'"
	}

	// build schema_migrations table data
	buf.WriteString("\n--\n-- Dbmate schema migrations\n--\n\n")

	if len(migrations) > 0 {
		buf.WriteString("INSERT INTO schema_migrations (version) VALUES\n    (" +
			strings.Join(migrations, "),\n    (") +
			");\n")
	}

	return nil
}

// DumpSchema returns the current database schema
func (drv ClickHouseDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	var buf bytes.Buffer
	var err error

	err = clickhouseSchemaDump(db, &buf, drv.databaseName(u))
	if err != nil {
		return nil, err
	}

	err = clickhouseSchemaMigrationsDump(db, &buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DatabaseExists determines whether the database exists
func (drv ClickHouseDriver) DatabaseExists(u *url.URL) (bool, error) {
	name := drv.databaseName(u)

	db, err := drv.openClickHouseDB(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow("SELECT 1 FROM system.databases where name = ?", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv ClickHouseDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		create table if not exists schema_migrations (
			version String,
			ts DateTime default now(),
			applied UInt8 default 1
		) engine = ReplacingMergeTree(ts)
		primary key version
		order by version
	`)
	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv ClickHouseDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := "select version from schema_migrations final where applied order by version desc"
	if limit >= 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}

	defer mustClose(rows)

	migrations := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = true
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv ClickHouseDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values (?)", version)
	return err
}

// DeleteMigration removes a migration record
func (drv ClickHouseDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec(
		"insert into schema_migrations (version, applied) values (?, ?)",
		version, false,
	)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv ClickHouseDriver) Ping(u *url.URL) error {
	// attempt connection to primary database, not "clickhouse" database
	// to support servers with no "clickhouse" database
	// (see https://github.com/amacneil/dbmate/issues/78)
	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	err = db.Ping()
	if err == nil {
		return nil
	}

	// ignore 'Database foo doesn't exist' error
	chErr, ok := err.(*clickhouse.Exception)
	if ok && chErr.Code == 81 {
		return nil
	}

	return err
}
