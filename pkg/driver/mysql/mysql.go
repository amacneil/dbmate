package mysql

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	_ "github.com/go-sql-driver/mysql" // database/sql driver
)

// for mocking out during tests
type execCmd interface {
	Output() ([]byte, error)
}

var execCommand = func(command string, args ...string) execCmd {
	return exec.Command(command, args...)
}
var execLookPath = exec.LookPath

func init() {
	dbmate.RegisterDriver(NewDriver, "mysql")
}

// Driver provides top level database functions
type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
}

// NewDriver initializes the driver
func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
	}
}

func connectionString(u *url.URL) string {
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
	userPass, _ := url.PathUnescape(userPassEncoded)

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

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("mysql", connectionString(drv.databaseURL))
}

func (drv *Driver) openRootDB() (*sql.DB, error) {
	// clone databaseURL
	rootURL, err := url.Parse(drv.databaseURL.String())
	if err != nil {
		return nil, err
	}

	// connect to no particular database
	rootURL.Path = "/"

	return sql.Open("mysql", connectionString(rootURL))
}

func (drv *Driver) quoteIdentifier(str string) string {
	str = strings.ReplaceAll(str, "`", "\\`")

	return fmt.Sprintf("`%s`", str)
}

// CreateDatabase creates the specified database
func (drv *Driver) CreateDatabase() error {
	name := dbutil.DatabaseName(drv.databaseURL)
	fmt.Fprintf(drv.log, "Creating: %s\n", name)

	db, err := drv.openRootDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	_, err = db.Exec(fmt.Sprintf("create database %s",
		drv.quoteIdentifier(name)))

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *Driver) DropDatabase() error {
	name := dbutil.DatabaseName(drv.databaseURL)
	fmt.Fprintf(drv.log, "Dropping: %s\n", name)

	db, err := drv.openRootDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	_, err = db.Exec(fmt.Sprintf("drop database if exists %s",
		drv.quoteIdentifier(name)))

	return err
}

type mysqldumpVersion struct {
	DbType  string  // "mysql" or "mariadb"
	Version float64 // major.minor version (e.g., 5.7, 8.4, 11.8)
	Command string  // "mysqldump" or "mariadb-dump"
}

var mysqldumpVersionRegexp = regexp.MustCompile(`(?:Ver \d+\.\d+ Distrib |Ver |from )(\d+\.\d+)`)

func getMysqldumpVersion() *mysqldumpVersion {
	ver := &mysqldumpVersion{
		DbType:  "mysql",
		Version: 5.0, // MariaDB 10.x is similar enough to MySQL 5.x
		Command: "mysqldump",
	}

	if _, err := execLookPath("mariadb-dump"); err == nil {
		// if we have mariadb-dump, we're at least MariaDB 11.x
		ver.DbType = "mariadb"
		ver.Version = 11.0
		ver.Command = "mariadb-dump"
	}

	cmd := execCommand(ver.Command, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ver
	}
	outputStr := string(output)

	if strings.Contains(outputStr, "MariaDB") {
		ver.DbType = "mariadb"
		ver.Version = 10.0
	}

	matches := mysqldumpVersionRegexp.FindStringSubmatch(outputStr)

	if len(matches) < 2 {
		return ver
	}

	version, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return ver
	}

	ver.Version = version

	return ver
}

func (drv *Driver) mysqldumpArgs(ver *mysqldumpVersion, userArgs ...string) []string {
	// generate CLI arguments
	args := []string{"--opt", "--routines", "--no-data",
		"--skip-dump-date", "--skip-add-drop-table"}

	tls := drv.databaseURL.Query().Get("tls")

	// Determine SSL flags based on database type and version
	useSSLMode := ver.DbType == "mysql" && ver.Version >= 8.0

	if tls == "" || strings.EqualFold(tls, "false") {
		if useSSLMode {
			args = append(args, "--ssl-mode=DISABLED")
		} else {
			args = append(args, "--ssl=false")
		}
	}
	if strings.EqualFold(tls, "skip-verify") {
		if useSSLMode {
			args = append(args, "--ssl-mode=PREFERRED")
		} else {
			args = append(args, "--ssl-verify-server-cert=false")
		}
	}
	if strings.EqualFold(tls, "true") && useSSLMode {
		args = append(args, "--ssl-mode=REQUIRED")
	}

	socket := drv.databaseURL.Query().Get("socket")
	if socket != "" {
		args = append(args, "--socket="+socket)
	} else {
		if hostname := drv.databaseURL.Hostname(); hostname != "" {
			args = append(args, "--host="+hostname)
		}
		if port := drv.databaseURL.Port(); port != "" {
			args = append(args, "--port="+port)
		}
	}

	if username := drv.databaseURL.User.Username(); username != "" {
		args = append(args, "--user="+username)
	}
	if password, set := drv.databaseURL.User.Password(); set {
		args = append(args, "--password="+password)
	}

	args = append(args, userArgs...)
	args = append(args, dbutil.DatabaseName(drv.databaseURL))
	return args
}

func (drv *Driver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := dbutil.QueryColumn(db,
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
func (drv *Driver) DumpSchema(db *sql.DB, extraArgs ...string) ([]byte, error) {
	ver := getMysqldumpVersion()

	schema, err := dbutil.RunCommand(ver.Command, drv.mysqldumpArgs(ver, extraArgs...)...)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	schema = append(schema, migrations...)
	schema, err = dbutil.TrimLeadingSQLComments(schema)
	if err != nil {
		return nil, err
	}
	return trimAutoincrementValues(schema), nil
}

// trimAutoincrementValues removes AUTO_INCREMENT values from MySQL schema dumps
func trimAutoincrementValues(data []byte) []byte {
	aiPattern := regexp.MustCompile(" AUTO_INCREMENT=[0-9]*")
	return aiPattern.ReplaceAll(data, []byte(""))
}

// DatabaseExists determines whether the database exists
func (drv *Driver) DatabaseExists() (bool, error) {
	name := dbutil.DatabaseName(drv.databaseURL)

	db, err := drv.openRootDB()
	if err != nil {
		return false, err
	}
	defer dbutil.MustClose(db)

	exists := false
	err = db.QueryRow("select true from information_schema.schemata "+
		"where schema_name = ?", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// MigrationsTableExists checks if the schema_migrations table exists
func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	match := ""
	err := db.QueryRow(fmt.Sprintf("show tables like '%s'",
		drv.migrationsTableName)).
		Scan(&match)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return match != "", err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf(
		"create table if not exists %s (version varchar(128) primary key)",
		drv.quotedMigrationsTableName()))

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s order by version desc", drv.quotedMigrationsTableName())
	if limit >= 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
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

// InsertMigration adds a new migration record
func (drv *Driver) InsertMigration(db dbutil.Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("insert into %s (version) values (?)", drv.quotedMigrationsTableName()),
		version)

	return err
}

// DeleteMigration removes a migration record
func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	_, err := db.Exec(
		fmt.Sprintf("delete from %s where version = ?", drv.quotedMigrationsTableName()),
		version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *Driver) Ping() error {
	db, err := drv.openRootDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	return db.Ping()
}

// Return a normalized version of the driver-specific error type.
func (drv *Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}

func (drv *Driver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}
