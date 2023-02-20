package dbmate

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/amacneil/dbmate/pkg/dbutil"
)

// Error codes
var (
	ErrNoMigrationFiles      = errors.New("no migration files found")
	ErrInvalidURL            = errors.New("invalid url, have you set your --url flag or DATABASE_URL environment variable?")
	ErrNoRollback            = errors.New("can't rollback: no migrations have been applied")
	ErrCantConnect           = errors.New("unable to connect to database")
	ErrUnsupportedDriver     = errors.New("unsupported driver")
	ErrNoMigrationName       = errors.New("please specify a name for the new migration")
	ErrMigrationAlreadyExist = errors.New("file already exists")
	ErrMigrationDirNotFound  = errors.New("could not find migrations directory")
	ErrMigrationNotFound     = errors.New("can't find migration file")
	ErrCreateDirectory       = errors.New("unable to create directory")
)

// migrationFileRegexp pattern for valid migration files
var migrationFileRegexp = regexp.MustCompile(`^\d.*\.sql$`)

// DB allows dbmate actions to be performed on a specified database
type DB struct {
	// AutoDumpSchema generates schema.sql after each action
	AutoDumpSchema bool
	// DatabaseURL is the database connection string
	DatabaseURL *url.URL
	// Log is the interface to write stdout
	Log io.Writer
	// MigrationsDir specifies the directory to find migration files
	MigrationsDir string
	// MigrationsTableName specifies the database table to record migrations in
	MigrationsTableName string
	// SchemaFile specifies the location for schema.sql file
	SchemaFile string
	// Verbose prints the result of each statement execution
	Verbose bool
	// WaitBefore will wait for database to become available before running any actions
	WaitBefore bool
	// WaitInterval specifies length of time between connection attempts
	WaitInterval time.Duration
	// WaitTimeout specifies maximum time for connection attempts
	WaitTimeout time.Duration
}

// StatusResult represents an available migration status
type StatusResult struct {
	Filename string
	Applied  bool
}

// New initializes a new dbmate database
func New(databaseURL *url.URL) *DB {
	return &DB{
		AutoDumpSchema:      true,
		DatabaseURL:         databaseURL,
		MigrationsDir:       "./db/migrations",
		MigrationsTableName: "schema_migrations",
		SchemaFile:          "./db/schema.sql",
		WaitBefore:          false,
		WaitInterval:        time.Second,
		WaitTimeout:         60 * time.Second,
		Log:                 os.Stdout,
	}
}

// Driver initializes the appropriate database driver
func (db *DB) Driver() (Driver, error) {
	if db.DatabaseURL == nil || db.DatabaseURL.Scheme == "" {
		return nil, ErrInvalidURL
	}

	driverFunc := drivers[db.DatabaseURL.Scheme]
	if driverFunc == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, db.DatabaseURL.Scheme)
	}

	config := DriverConfig{
		DatabaseURL:         db.DatabaseURL,
		Log:                 db.Log,
		MigrationsTableName: db.MigrationsTableName,
	}
	drv := driverFunc(config)

	if db.WaitBefore {
		if err := db.wait(drv); err != nil {
			return nil, err
		}
	}

	return drv, nil
}

func (db *DB) wait(drv Driver) error {
	// attempt connection to database server
	err := drv.Ping()
	if err == nil {
		// connection successful
		return nil
	}

	fmt.Fprint(db.Log, "Waiting for database")
	for i := 0 * time.Second; i < db.WaitTimeout; i += db.WaitInterval {
		fmt.Fprint(db.Log, ".")
		time.Sleep(db.WaitInterval)

		// attempt connection to database server
		err = drv.Ping()
		if err == nil {
			// connection successful
			fmt.Fprint(db.Log, "\n")
			return nil
		}
	}

	// if we find outselves here, we could not connect within the timeout
	fmt.Fprint(db.Log, "\n")
	return fmt.Errorf("%w: %s", ErrCantConnect, err)
}

// Wait blocks until the database server is available. It does not verify that
// the specified database exists, only that the host is ready to accept connections.
func (db *DB) Wait() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	// if db.WaitBefore is true, wait() will get called twice, no harm
	return db.wait(drv)
}

// CreateAndMigrate creates the database (if necessary) and runs migrations
func (db *DB) CreateAndMigrate() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	// create database if it does not already exist
	// skip this step if we cannot determine status
	// (e.g. user does not have list database permission)
	exists, err := drv.DatabaseExists()
	if err == nil && !exists {
		if err := drv.CreateDatabase(); err != nil {
			return err
		}
	}

	// migrate
	return db.Migrate()
}

// Create creates the current database
func (db *DB) Create() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	return drv.CreateDatabase()
}

// Drop drops the current database (if it exists)
func (db *DB) Drop() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	return drv.DropDatabase()
}

// DumpSchema writes the current database schema to a file
func (db *DB) DumpSchema() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	sqlDB, err := db.openDatabaseForMigration(drv)
	if err != nil {
		return err
	}
	defer dbutil.MustClose(sqlDB)

	schema, err := drv.DumpSchema(sqlDB)
	if err != nil {
		return err
	}

	fmt.Fprintf(db.Log, "Writing: %s\n", db.SchemaFile)

	// ensure schema directory exists
	if err = ensureDir(filepath.Dir(db.SchemaFile)); err != nil {
		return err
	}

	// write schema to file
	return os.WriteFile(db.SchemaFile, schema, 0o644)
}

// ensureDir creates a directory if it does not already exist
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w `%s`", ErrCreateDirectory, dir)
	}

	return nil
}

const migrationTemplate = "-- migrate:up\n\n\n-- migrate:down\n\n"

// NewMigration creates a new migration file
func (db *DB) NewMigration(name string) error {
	// new migration name
	timestamp := time.Now().UTC().Format("20060102150405")
	if name == "" {
		return ErrNoMigrationName
	}
	name = fmt.Sprintf("%s_%s.sql", timestamp, name)

	// create migrations dir if missing
	if err := ensureDir(db.MigrationsDir); err != nil {
		return err
	}

	// check file does not already exist
	path := filepath.Join(db.MigrationsDir, name)
	fmt.Fprintf(db.Log, "Creating migration: %s\n", path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return ErrMigrationAlreadyExist
	}

	// write new migration
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer dbutil.MustClose(file)
	_, err = file.WriteString(migrationTemplate)
	return err
}

func doTransaction(sqlDB *sql.DB, txFunc func(dbutil.Transaction) error) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}

	if err := txFunc(tx); err != nil {
		if err1 := tx.Rollback(); err1 != nil {
			return err1
		}

		return err
	}

	return tx.Commit()
}

func (db *DB) openDatabaseForMigration(drv Driver) (*sql.DB, error) {
	sqlDB, err := drv.Open()
	if err != nil {
		return nil, err
	}

	if err := drv.CreateMigrationsTable(sqlDB); err != nil {
		dbutil.MustClose(sqlDB)
		return nil, err
	}

	return sqlDB, nil
}

// Migrate migrates database to the latest version
func (db *DB) Migrate() error {
	files, err := findMigrationFiles(db.MigrationsDir, migrationFileRegexp)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return ErrNoMigrationFiles
	}

	drv, err := db.Driver()
	if err != nil {
		return err
	}

	sqlDB, err := db.openDatabaseForMigration(drv)
	if err != nil {
		return err
	}
	defer dbutil.MustClose(sqlDB)

	applied, err := drv.SelectMigrations(sqlDB, -1)
	if err != nil {
		return err
	}

	for _, filename := range files {
		ver := migrationVersion(filename)
		if ok := applied[ver]; ok {
			// migration already applied
			continue
		}

		fmt.Fprintf(db.Log, "Applying: %s\n", filename)

		up, _, err := parseMigration(filepath.Join(db.MigrationsDir, filename))
		if err != nil {
			return err
		}

		execMigration := func(tx dbutil.Transaction) error {
			// run actual migration
			result, err := tx.Exec(up.Contents)
			if err != nil {
				return err
			} else if db.Verbose {
				db.printVerbose(result)
			}

			// record migration
			return drv.InsertMigration(tx, ver)
		}

		if up.Options.Transaction() {
			// begin transaction
			err = doTransaction(sqlDB, execMigration)
		} else {
			// run outside of transaction
			err = execMigration(sqlDB)
		}

		if err != nil {
			return err
		}
	}

	// automatically update schema file, silence errors
	if db.AutoDumpSchema {
		_ = db.DumpSchema()
	}

	return nil
}

func (db *DB) printVerbose(result sql.Result) {
	lastInsertID, err := result.LastInsertId()
	if err == nil {
		fmt.Fprintf(db.Log, "Last insert ID: %d\n", lastInsertID)
	}
	rowsAffected, err := result.RowsAffected()
	if err == nil {
		fmt.Fprintf(db.Log, "Rows affected: %d\n", rowsAffected)
	}
}

func findMigrationFiles(dir string, re *regexp.Regexp) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w `%s`", ErrMigrationDirNotFound, dir)
	}

	matches := []string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !re.MatchString(name) {
			continue
		}

		matches = append(matches, name)
	}

	sort.Strings(matches)

	return matches, nil
}

func findMigrationFile(dir string, ver string) (string, error) {
	if ver == "" {
		panic("migration version is required")
	}

	ver = regexp.QuoteMeta(ver)
	re := regexp.MustCompile(fmt.Sprintf(`^%s.*\.sql$`, ver))

	files, err := findMigrationFiles(dir, re)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("%w: %s*.sql", ErrMigrationNotFound, ver)
	}

	return files[0], nil
}

func migrationVersion(filename string) string {
	return regexp.MustCompile(`^\d+`).FindString(filename)
}

// Rollback rolls back the most recent migration
func (db *DB) Rollback() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	sqlDB, err := db.openDatabaseForMigration(drv)
	if err != nil {
		return err
	}
	defer dbutil.MustClose(sqlDB)

	applied, err := drv.SelectMigrations(sqlDB, 1)
	if err != nil {
		return err
	}

	// grab most recent applied migration (applied has len=1)
	latest := ""
	for ver := range applied {
		latest = ver
	}
	if latest == "" {
		return ErrNoRollback
	}

	filename, err := findMigrationFile(db.MigrationsDir, latest)
	if err != nil {
		return err
	}

	fmt.Fprintf(db.Log, "Rolling back: %s\n", filename)

	_, down, err := parseMigration(filepath.Join(db.MigrationsDir, filename))
	if err != nil {
		return err
	}

	execMigration := func(tx dbutil.Transaction) error {
		// rollback migration
		result, err := tx.Exec(down.Contents)
		if err != nil {
			return err
		} else if db.Verbose {
			db.printVerbose(result)
		}

		// remove migration record
		return drv.DeleteMigration(tx, latest)
	}

	if down.Options.Transaction() {
		// begin transaction
		err = doTransaction(sqlDB, execMigration)
	} else {
		// run outside of transaction
		err = execMigration(sqlDB)
	}

	if err != nil {
		return err
	}

	// automatically update schema file, silence errors
	if db.AutoDumpSchema {
		_ = db.DumpSchema()
	}

	return nil
}

// Status shows the status of all migrations
func (db *DB) Status(quiet bool) (int, error) {
	results, err := db.CheckMigrationsStatus()
	if err != nil {
		return -1, err
	}

	var totalApplied int
	var line string

	for _, res := range results {
		if res.Applied {
			line = fmt.Sprintf("[X] %s", res.Filename)
			totalApplied++
		} else {
			line = fmt.Sprintf("[ ] %s", res.Filename)
		}
		if !quiet {
			fmt.Fprintln(db.Log, line)
		}
	}

	totalPending := len(results) - totalApplied
	if !quiet {
		fmt.Fprintln(db.Log)
		fmt.Fprintf(db.Log, "Applied: %d\n", totalApplied)
		fmt.Fprintf(db.Log, "Pending: %d\n", totalPending)
	}

	return totalPending, nil
}

// CheckMigrationsStatus returns the status of all available mgirations
func (db *DB) CheckMigrationsStatus() ([]StatusResult, error) {
	drv, err := db.Driver()
	if err != nil {
		return nil, err
	}

	files, err := findMigrationFiles(db.MigrationsDir, migrationFileRegexp)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, ErrNoMigrationFiles
	}

	sqlDB, err := drv.Open()
	if err != nil {
		return nil, err
	}
	defer dbutil.MustClose(sqlDB)

	applied := map[string]bool{}

	migrationsTableExists, err := drv.MigrationsTableExists(sqlDB)
	if err != nil {
		return nil, err
	}

	if migrationsTableExists {
		applied, err = drv.SelectMigrations(sqlDB, -1)
		if err != nil {
			return nil, err
		}
	}

	var results []StatusResult

	for _, filename := range files {
		ver := migrationVersion(filename)
		res := StatusResult{Filename: filename}
		if ok := applied[ver]; ok {
			res.Applied = true
		} else {
			res.Applied = false
		}

		results = append(results, res)
	}

	return results, nil
}
