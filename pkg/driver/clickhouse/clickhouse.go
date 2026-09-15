package clickhouse

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func init() {
	dbmate.RegisterDriver(NewDriver, "clickhouse")
	dbmate.RegisterDriver(NewDriver, "clickhouse+http")
	dbmate.RegisterDriver(NewDriver, "clickhouse+https")
}

// Driver provides top level database functions
type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
	clusterParameters   *ClusterParameters
}

// NewDriver initializes the driver
func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
		clusterParameters:   ExtractClusterParametersFromURL(config.DatabaseURL),
	}
}

func connectionString(initialURL *url.URL) string {
	// clone url
	u, _ := url.Parse(initialURL.String())

	// Default TCP port for Clickhouse (works both for tcp and clickhouse scheme)
	defaultPort := "9000"

	// Update scheme and default port based on variant
	switch u.Scheme {
	case "clickhouse+http", "http":
		u.Scheme = "http"
		defaultPort = "8123"
	case "clickhouse+https", "https":
		u.Scheme = "https"
		defaultPort = "8443"
	}

	// Set default port if not specified
	if u.Port() == "" {
		u.Host = fmt.Sprintf("%s:%s", u.Host, defaultPort)
	}

	query := u.Query()
	username := u.User.Username()
	password, _ := u.User.Password()

	if query.Get("username") != "" {
		username = query.Get("username")
		query.Del("username")
	}
	if query.Get("password") != "" {
		password = query.Get("password")
		query.Del("password")
	}

	if username != "" {
		if password == "" {
			u.User = url.User(username)
		} else {
			u.User = url.UserPassword(username, password)
		}
	}

	if query.Get("database") != "" {
		u.Path = fmt.Sprintf("/%s", query.Get("database"))
		query.Del("database")
	}

	u.RawQuery = query.Encode()

	u = ClearClusterParametersFromURL(u)

	return u.String()
}

// Open creates a new database connection
func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("clickhouse", connectionString(drv.databaseURL))
}

func (drv *Driver) openClickHouseDB() (*sql.DB, error) {
	// clone databaseURL
	clickhouseURL, err := url.Parse(connectionString(drv.databaseURL))
	if err != nil {
		return nil, err
	}

	// connect to clickhouse database
	clickhouseURL.Path = "/default"

	return sql.Open("clickhouse", clickhouseURL.String())
}

func (drv *Driver) onClusterClause() string {
	clusterClause := ""
	if drv.clusterParameters.OnCluster {
		escapedClusterMacro := drv.escapeString(drv.clusterParameters.ClusterMacro)
		clusterClause = fmt.Sprintf(" ON CLUSTER '%s'", escapedClusterMacro)
	}
	return clusterClause
}

func (drv *Driver) databaseName() string {
	u, err := url.Parse(connectionString(drv.databaseURL))
	if err != nil {
		panic(err)
	}

	name := strings.TrimLeft(u.Path, "/")
	if name == "" {
		name = "default"
	}
	return name
}

var clickhouseValidIdentifier = regexp.MustCompile(`^[a-zA-Z_][0-9a-zA-Z_]*$`)

func (drv *Driver) quoteIdentifier(str string) string {
	if clickhouseValidIdentifier.MatchString(str) {
		return str
	}

	str = strings.ReplaceAll(str, `"`, `""`)

	return fmt.Sprintf(`"%s"`, str)
}

func (drv *Driver) escapeString(str string) string {
	quoteEscaper := strings.NewReplacer(`'`, `\'`, `\`, `\\`)
	str = quoteEscaper.Replace(str)
	return str
}

// CreateDatabase creates the specified database
func (drv *Driver) CreateDatabase() error {
	databaseName := drv.quotedDatabaseName()
	fmt.Fprintf(drv.log, "Creating: %s\n", databaseName)

	db, err := drv.openClickHouseDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	q := fmt.Sprintf("CREATE DATABASE %s%s", databaseName, drv.onClusterClause())

	_, err = db.Exec(q)

	return err
}

// DropDatabase drops the specified database (if it exists)
func (drv *Driver) DropDatabase() error {
	databaseName := drv.quotedDatabaseName()
	fmt.Fprintf(drv.log, "Dropping: %s\n", databaseName)

	db, err := drv.openClickHouseDB()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	q := fmt.Sprintf("DROP DATABASE IF EXISTS %s%s", databaseName, drv.onClusterClause())

	_, err = db.Exec(q)

	return err
}

func (drv *Driver) schemaDump(db *sql.DB, buf *bytes.Buffer) error {
	buf.WriteString("\n--\n-- Database schema\n--\n\n")
	fmt.Fprintf(buf, "CREATE DATABASE IF NOT EXISTS %s%s;\n\n", drv.quotedDatabaseName(), drv.onClusterClause())

	tables, err := dbutil.QueryColumn(db, "show tables")
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

func (drv *Driver) schemaMigrationsDump(db *sql.DB, buf *bytes.Buffer) error {
	migrationsTable := drv.quotedMigrationsTableName()

	// load applied migrations
	migrations, err := dbutil.QueryColumn(db,
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
			fmt.Sprintf("INSERT INTO %s.%s (version) VALUES\n    (", drv.quotedDatabaseName(), migrationsTable) +
				strings.Join(migrations, "),\n    (") +
				");\n")
	}

	return nil
}

// DumpSchema returns the current database schema
func (drv *Driver) DumpSchema(db *sql.DB, _ ...string) ([]byte, error) {
	var buf bytes.Buffer
	var err error

	err = drv.schemaDump(db, &buf)
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
func (drv *Driver) DatabaseExists() (bool, error) {
	name := drv.databaseName()

	db, err := drv.openClickHouseDB()
	if err != nil {
		return false, err
	}
	defer dbutil.MustClose(db)

	exists := false
	err = db.QueryRow("SELECT 1 FROM system.databases where name = ?", name).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// MigrationsTableExists checks if the schema_migrations table exists
func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	exists := false
	err := db.QueryRow(fmt.Sprintf("EXISTS TABLE %s", drv.quotedMigrationsTableName())).
		Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists, err
}

// CreateMigrationsTable creates the schema migrations table
func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	engineClause := "ReplacingMergeTree(ts)"
	if drv.clusterParameters.OnCluster {
		escapedZooPath := drv.escapeString(drv.clusterParameters.ZooPath)
		escapedReplicaMacro := drv.escapeString(drv.clusterParameters.ReplicaMacro)
		engineClause = fmt.Sprintf("ReplicatedReplacingMergeTree('%s', '%s', ts)", escapedZooPath, escapedReplicaMacro)
	}

	_, err := db.Exec(fmt.Sprintf(`
		create table if not exists %s%s (
			version String,
			ts DateTime default now(),
			applied UInt8 default 1
		) engine = %s
		primary key version
		order by version
	`, drv.quotedMigrationsTableName(), drv.onClusterClause(), engineClause))

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	query := fmt.Sprintf("select version from %s final where applied order by version desc",
		drv.quotedMigrationsTableName())

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
		fmt.Sprintf("insert into %s (version, applied) values (?, ?)",
			drv.quotedMigrationsTableName()),
		version, false,
	)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv *Driver) Ping() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

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

// Return a normalized version of the driver-specific error type.
func (drv *Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}

func (drv *Driver) quotedMigrationsTableName() string {
	return drv.quoteIdentifier(drv.migrationsTableName)
}

func (drv *Driver) quotedDatabaseName() string {
	return drv.quoteIdentifier(drv.databaseName())
}
