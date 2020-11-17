package dbmate

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/go-sql-driver/mysql" // mysql driver for database/sql
)

func init() {
	RegisterDriver(&MySQLDriver{}, "mysql")
}

// MySQLDriver provides top level database functions
type MySQLDriver struct {
	migrationsTableName string
}

func normalizeMySQLURL(u *url.URL) string {
	query := u.Query()
	query.Set("multiStatements", "true")

	host := u.Host
	protocol := "tcp"

	if query.Get("socket") != "" {
		protocol = "unix"
		host = query.Get("socket")
		query.Del("socket")
	} else if u.Port() == "" {
		// set default port
		host = fmt.Sprintf("%s:3306", host)
	}

	// Get decoded user:pass
	userPassEncoded := u.User.String()
	userPass, _ := url.QueryUnescape(userPassEncoded)

	// Build DSN w/ user:pass percent-decoded
	normalizedString := ""

	if userPass != "" { // user:pass can be empty
		normalizedString = userPass + "@"
	}

	// connection string format required by go-sql-driver/mysql
	normalizedString = fmt.Sprintf("%s%s(%s)%s?%s", normalizedString,
		protocol, host, u.Path, query.Encode())

	return normalizedString
}

// SetMigrationsTableName sets the schema migrations table name
func (drv *MySQLDriver) SetMigrationsTableName(name string) {
	drv.migrationsTableName = name
}

// Open creates a new database connection
func (drv *MySQLDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("mysql", normalizeMySQLURL(u))
}

func (drv *MySQLDriver) openRootDB(u *url.URL) (*sql.DB, error) {
	// connect to no particular database
	rootURL := *u
	rootURL.Path = "/"

	return drv.Open(&rootURL)
}

func (drv *MySQLDriver) quoteIdentifier(str string) string {
	str = strings.Replace(str, "`", "\\`", -1)

	return fmt.Sprintf("`%s`", str)
}

// CreateDatabase creates the specified database
func (drv *MySQLDriver) CreateDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		drv.quoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *MySQLDriver) DropDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		drv.quoteIdentifier(name)))

	return err
}

func (drv *MySQLDriver) mysqldumpArgs(u *url.URL) []string {
	// generate CLI arguments
	args := []string{"--opt", "--routines", "--no-data",
		"--skip-dump-date", "--skip-add-drop-table"}

	if hostname := u.Hostname(); hostname != "" {
		args = append(args, "--host="+hostname)
	}
	if port := u.Port(); port != "" {
		args = append(args, "--port="+port)
	}
	if username := u.User.Username(); username != "" {
		args = append(args, "--user="+username)
	}
	// mysql recommends against using environment variables to supply password
	// https://dev.mysql.com/doc/refman/5.7/en/password-security-user.html
	if password, set := u.User.Password(); set {
		args = append(args, "--password="+password)
	}

	// add database name
	args = append(args, strings.TrimLeft(u.Path, "/"))

	return args
}

func (drv *MySQLDriver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := queryColumn(db,
		fmt.Sprintf("select quote(version) from %s order by version asc", migrationsTable))
	if err != nil {
		return nil, err
	}

	// build schema_migrations table data
	var buf bytes.Buffer
	buf.WriteString("\n--\n-- Dbmate schema migrations\n--\n\n" +
		fmt.Sprintf("LOCK TABLES %s WRITE;\n", migrationsTable))

	if len(migrations) > 0 {
		buf.WriteString(
			fmt.Sprintf("INSERT INTO %s (version) VALUES\n  (", migrationsTable) +
				strings.Join(migrations, "),\n  (") +
				");\n")
	}

	buf.WriteString("UNLOCK TABLES;\n")

	return buf.Bytes(), nil
}

// DumpSchema returns the current database schema
func (drv *MySQLDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	schema, err := runCommand("mysqldump", drv.mysqldumpArgs(u)...)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	schema = append(schema, migrations...)
	return trimLeadingSQLComments(schema)
}

// DatabaseExists determines whether the database exists
func (drv *MySQLDriver) DatabaseExists(u *url.URL) (bool, error) {
	name := databaseName(u)

	db, err := drv.openRootDB(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	exists := false
	err = db.QueryRow("select true from information_schema.schemata "+
		"where schema_name = ?", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv *MySQLDriver) CreateMigrationsTable(u *url.URL, db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf("create table if not exists %s "+
		"(version varchar(255) primary key) character set latin1 collate latin1_bin",
		drv.quotedMigrationsTableName()))

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *MySQLDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s order by version desc", drv.quotedMigrationsTableName())
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
func (drv *MySQLDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version) values (?)", drv.quotedMigrationsTableName()),
		version)

	return err
}

// DeleteMigration removes a migration record
func (drv *MySQLDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("delete from %s where version = ?", drv.quotedMigrationsTableName()),
		version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *MySQLDriver) Ping(u *url.URL) error {
	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	return db.Ping()
}

func (drv *MySQLDriver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}
