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
	RegisterDriver(newClickHouseDriver, "clickhouse")
}

// ClickHouseDriver provides top level database functions
type ClickHouseDriver struct {
	migrationsTableName string
	databaseURL         *url.URL
}

func newClickHouseDriver(config DriverConfig) Driver {
	return &ClickHouseDriver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
	}
}

func normalizeClickHouseURL(initialURL *url.URL) *url.URL {
	u := *initialURL

	u.Scheme = "tcp"
	host := u.Host
	if u.Port() == "" {
		host = fmt.Sprintf("%s:9000", host)
	}
	u.Host = host

	query := u.Query()
	if query.Get("username") == "" && u.User.Username() != "" {
		query.Set("username", u.User.Username())
	}
	password, passwordSet := u.User.Password()
	if query.Get("password") == "" && passwordSet {
		query.Set("password", password)
	}
	u.User = nil

	if query.Get("database") == "" {
		path := strings.Trim(u.Path, "/")
		if path != "" {
			query.Set("database", path)
			u.Path = ""
		}
	}
	u.RawQuery = query.Encode()

	return &u
}

// Open creates a new database connection
func (drv *ClickHouseDriver) Open() (*sql.DB, error) {
	return sql.Open("clickhouse", normalizeClickHouseURL(drv.databaseURL).String())
}

func (drv *ClickHouseDriver) openClickHouseDB() (*sql.DB, error) {
	// clone databaseURL
	clickhouseURL, err := url.Parse(normalizeClickHouseURL(drv.databaseURL).String())
	if err != nil {
		return nil, err
	}

	// connect to clickhouse database
	values := clickhouseURL.Query()
	values.Set("database", "default")
	clickhouseURL.RawQuery = values.Encode()

	return sql.Open("clickhouse", clickhouseURL.String())
}

func (drv *ClickHouseDriver) databaseName() string {
	name := normalizeClickHouseURL(drv.databaseURL).Query().Get("database")
	if name == "" {
		name = "default"
	}
	return name
}

var clickhouseValidIdentifier = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]*$`)

func (drv *ClickHouseDriver) quoteIdentifier(str string) string {
	if clickhouseValidIdentifier.MatchString(str) {
		return str
	}

	str = strings.Replace(str, `"`, `""`, -1)

	return fmt.Sprintf(`"%s"`, str)
}

// CreateDatabase creates the specified database
func (drv *ClickHouseDriver) CreateDatabase() error {
	name := drv.databaseName()
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openClickHouseDB()
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec("create database " + drv.quoteIdentifier(name))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *ClickHouseDriver) DropDatabase() error {
	name := drv.databaseName()
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openClickHouseDB()
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec("drop database if exists " + drv.quoteIdentifier(name))

	return err
}

func (drv *ClickHouseDriver) schemaDump(db *sql.DB, buf *bytes.Buffer, databaseName string) error {
	buf.WriteString("\n--\n-- Database schema\n--\n\n")

	buf.WriteString("CREATE DATABASE " + drv.quoteIdentifier(databaseName) + " IF NOT EXISTS;\n\n")

	tables, err := queryColumn(db, "show tables")
	if err != nil {
		return err
	}
	sort.Strings(tables)

	for _, table := range tables {
		var clause string
		err = db.QueryRow("show create table " + drv.quoteIdentifier(table)).Scan(&clause)
		if err != nil {
			return err
		}
		buf.WriteString(clause + ";\n\n")
	}
	return nil
}

func (drv *ClickHouseDriver) schemaMigrationsDump(db *sql.DB, buf *bytes.Buffer) error {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := queryColumn(db,
		fmt.Sprintf("select version from %s final ", migrationsTable)+
			"where applied order by version asc",
	)
	if err != nil {
		return err
	}

	quoter := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	for i := range migrations {
		migrations[i] = "'" + quoter.Replace(migrations[i]) + "'"
	}

	// build schema migrations table data
	buf.WriteString("\n--\n-- Dbmate schema migrations\n--\n\n")

	if len(migrations) > 0 {
		buf.WriteString(
			fmt.Sprintf("INSERT INTO %s (version) VALUES\n    (", migrationsTable) +
				strings.Join(migrations, "),\n    (") +
				");\n")
	}

	return nil
}

// DumpSchema returns the current database schema
func (drv *ClickHouseDriver) DumpSchema(db *sql.DB) ([]byte, error) {
	var buf bytes.Buffer
	var err error

	err = drv.schemaDump(db, &buf, drv.databaseName())
	if err != nil {
		return nil, err
	}

	err = drv.schemaMigrationsDump(db, &buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DatabaseExists determines whether the database exists
func (drv *ClickHouseDriver) DatabaseExists() (bool, error) {
	name := drv.databaseName()

	db, err := drv.openClickHouseDB()
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

// CreateMigrationsTable creates the schema migrations table
func (drv *ClickHouseDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf(`
		create table if not exists %s (
			version String,
			ts DateTime default now(),
			applied UInt8 default 1
		) engine = ReplacingMergeTree(ts)
		primary key version
		order by version
	`, drv.quotedMigrationsTableName()))

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *ClickHouseDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s final where applied order by version desc",
		drv.quotedMigrationsTableName())

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

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv *ClickHouseDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version) values (?)", drv.quotedMigrationsTableName()),
		version)

	return err
}

// DeleteMigration removes a migration record
func (drv *ClickHouseDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version, applied) values (?, ?)",
			drv.quotedMigrationsTableName()),
		version, false,
	)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *ClickHouseDriver) Ping() error {
	// attempt connection to primary database, not "clickhouse" database
	// to support servers with no "clickhouse" database
	// (see https://github.com/amacneil/dbmate/issues/78)
	db, err := drv.Open()
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

func (drv *ClickHouseDriver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}
