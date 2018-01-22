package dbmate

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql" // mysql driver for database/sql
)

// MySQLDriver provides top level database functions
type MySQLDriver struct {
}

func normalizeMySQLURL(u *url.URL) string {
	normalizedURL := *u
	normalizedURL.Scheme = ""

	// set default port
	if normalizedURL.Port() == "" {
		normalizedURL.Host = fmt.Sprintf("%s:3306", normalizedURL.Host)
	}

	// host format required by go-sql-driver/mysql
	normalizedURL.Host = fmt.Sprintf("tcp(%s)", normalizedURL.Host)

	query := normalizedURL.Query()
	query.Set("multiStatements", "true")
	normalizedURL.RawQuery = query.Encode()

	str := normalizedURL.String()
	return strings.TrimLeft(str, "/")
}

// Open creates a new database connection
func (drv MySQLDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open("mysql", normalizeMySQLURL(u))
}

func (drv MySQLDriver) openRootDB(u *url.URL) (*sql.DB, error) {
	// connect to no particular database
	rootURL := *u
	rootURL.Path = "/"

	return drv.Open(&rootURL)
}

func quoteIdentifier(str string) string {
	str = strings.Replace(str, "`", "\\`", -1)

	return fmt.Sprintf("`%s`", str)
}

// CreateDatabase creates the specified database
func (drv MySQLDriver) CreateDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Creating: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		quoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv MySQLDriver) DropDatabase(u *url.URL) error {
	name := databaseName(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openRootDB(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		quoteIdentifier(name)))

	return err
}

// DumpSchema returns the current database schema
func (drv MySQLDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	// generate CLI arguments
	args := []string{"--opt", "--no-data", "--skip-dump-date", "--skip-add-drop-table"}

	if hostname := u.Hostname(); hostname != "" {
		args = append(args, "--host="+hostname)
	}
	if port := u.Port(); port != "" {
		args = append(args, "--port="+port)
	}
	if username := u.User.Username(); username != "" {
		args = append(args, "--user="+username)
	}
	// mysql recommands against using environment variables to supply password
	// https://dev.mysql.com/doc/refman/5.7/en/password-security-user.html
	// a potentially more secure way to do this would be to write a temporary file
	if password, set := u.User.Password(); set {
		args = append(args, "--password="+password)
	}

	// add database name
	args = append(args, strings.TrimLeft(u.Path, "/"))

	schema, err := runCommand("mysqldump", args...)
	if err != nil {
		return nil, err
	}

	// load applied migrations
	rows, err := db.Query("select quote(version) from schema_migrations order by version desc")
	if err != nil {
		return nil, err
	}
	defer mustClose(rows)

	// build schema_migrations table data
	var buf bytes.Buffer
	buf.WriteString("\n--\n-- Dbmate applied migrations\n--\n\n" +
		"LOCK TABLES `schema_migrations` WRITE;\n" +
		"/*!40000 ALTER TABLE `schema_migrations` DISABLE KEYS */;\n")

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		buf.WriteString(
			fmt.Sprintf("INSERT INTO `schema_migrations` VALUES (%s);\n", version))
	}
	buf.WriteString("/*!40000 ALTER TABLE `schema_migrations` ENABLE KEYS */;\n" +
		"UNLOCK TABLES;\n\n")

	// insert migrations table data before client settings are restored
	re := regexp.MustCompile(`(?m)^.*SET TIME_ZONE=@OLD_TIME_ZONE.*$`)
	matched := false
	schema = re.ReplaceAllFunc(schema, func(match []byte) []byte {
		// match only once
		if matched {
			return match
		}

		matched = true
		return append(buf.Bytes(), match...)
	})

	return schema, nil
}

// DatabaseExists determines whether the database exists
func (drv MySQLDriver) DatabaseExists(u *url.URL) (bool, error) {
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
func (drv MySQLDriver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec("create table if not exists schema_migrations " +
		"(version varchar(255) primary key)")

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv MySQLDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := "select version from schema_migrations order by version desc"
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
func (drv MySQLDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values (?)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv MySQLDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = ?", version)

	return err
}
