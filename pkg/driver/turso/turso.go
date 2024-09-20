package turso

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func init() {
	dbmate.RegisterDriver(NewDriver, "turso")
}

type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
}

func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
	}
}

func connectionString(u *url.URL) string {
	newURL := *u
	newURL.Scheme = "libsql"
	return newURL.String()
}

func (drv *Driver) CreateDatabase() error {
	return fmt.Errorf("Please use turso-cli to create database")
}

func (drv *Driver) DropDatabase() error {
	return fmt.Errorf("Please use turso-cli to drop database")
}

func (drv *Driver) DatabaseExists() (bool, error) {
	err := drv.Ping()
	if err != nil {
		return false, err
	}
	return true, nil
}

func (drv *Driver) Ping() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)
	return db.Ping()
}

func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("libsql", connectionString(drv.databaseURL))
}

func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	exists := false
	query := "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?"
	err := db.QueryRow(query, drv.migrationsTableName).Scan(&exists)
	if err != nil {
		fmt.Println("Error on checking if table exists", err)
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return exists, err
}

func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	fmt.Println("Creating migration table", drv.migrationsTableName)
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (version TEXT PRIMARY KEY)",
		drv.migrationsTableName,
	)
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println("Failed to crete migration table", err)
	}
	return err
}

func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("SELECT version FROM %s ORDER BY version DESC", drv.migrationsTableName)
	if limit >= 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}

	defer dbutil.MustClose(rows)

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

func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE version = ?", drv.migrationsTableName)
	_, err := db.Exec(query, version)
	return err
}

func (drv *Driver) InsertMigration(db dbutil.Transaction, version string) error {
	query := fmt.Sprintf("INSERT INTO %s (version) VALUES (?)", drv.migrationsTableName)
	_, err := db.Exec(query, version)
	return err
}

func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
	path := connectionString(drv.databaseURL)
	schema, err := dbutil.RunCommand("turso", "db", "shell", path, ".schema")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	schema = append(schema, migrations...)
	return dbutil.TrimLeadingSQLComments(schema)
}

func (drv *Driver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	fmt.Println("schema migrations dump")
	// load applied migrations
	query := fmt.Sprintf(
		"SELECT quote(version) FROM %s order by version asc",
		drv.migrationsTableName,
	)
	migrations, err := dbutil.QueryColumn(db, query)
	if err != nil {
		return nil, err
	}

	// build schema migrations table data
	var buf bytes.Buffer
	buf.WriteString("-- Dbmate schema migrations\n")
	if len(migrations) > 0 {
		buf.WriteString(
			fmt.Sprintf("INSERT INTO %s (version) VALUES\n  (", drv.migrationsTableName) +
				strings.Join(migrations, "),\n  (") +
				");\n")
	}

	return buf.Bytes(), nil
}

func (drv *Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}
