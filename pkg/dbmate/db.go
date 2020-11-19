package dbmate

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/amacneil/dbmate/pkg/dbutil"
)

// DefaultMigrationsDir specifies default directory to find migration files
const DefaultMigrationsDir = "./db/migrations"

// DefaultMigrationsTableName specifies default database tables to record migraitons in
const DefaultMigrationsTableName = "schema_migrations"

// DefaultSchemaFile specifies default location for schema.sql
const DefaultSchemaFile = "./db/schema.sql"

// DefaultWaitInterval specifies length of time between connection attempts
const DefaultWaitInterval = time.Second

// DefaultWaitTimeout specifies maximum time for connection attempts
const DefaultWaitTimeout = 60 * time.Second

// DB allows dbmate actions to be performed on a specified database
type DB struct {
	AutoDumpSchema      bool
	DatabaseURL         *url.URL
	MigrationsDir       string
	MigrationsTableName string
	SchemaFile          string
	Verbose             bool
	WaitBefore          bool
	WaitInterval        time.Duration
	WaitTimeout         time.Duration
}

// migrationFileRegexp pattern for valid migration files
var migrationFileRegexp = regexp.MustCompile(`^\d.*\.sql$`)

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
		MigrationsDir:       DefaultMigrationsDir,
		MigrationsTableName: DefaultMigrationsTableName,
		SchemaFile:          DefaultSchemaFile,
		WaitBefore:          false,
		WaitInterval:        DefaultWaitInterval,
		WaitTimeout:         DefaultWaitTimeout,
	}
}

// GetDriver initializes the appropriate database driver
func (db *DB) GetDriver() (Driver, error) {
	if db.DatabaseURL == nil || db.DatabaseURL.Scheme == "" {
		return nil, errors.New("invalid url")
	}

	driverFunc := drivers[db.DatabaseURL.Scheme]
	if driverFunc == nil {
		return nil, fmt.Errorf("unsupported driver: %s", db.DatabaseURL.Scheme)
	}

	config := DriverConfig{
		DatabaseURL:         db.DatabaseURL,
		MigrationsTableName: db.MigrationsTableName,
	}

	return driverFunc(config), nil
}

// Wait blocks until the database server is available. It does not verify that
// the specified database exists, only that the host is ready to accept connections.
func (db *DB) Wait() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	return db.wait(drv)
}

func (db *DB) wait(drv Driver) error {
	// attempt connection to database server
	err := drv.Ping()
	if err == nil {
		// connection successful
		return nil
	}

	fmt.Print("Waiting for database")
	for i := 0 * time.Second; i < db.WaitTimeout; i += db.WaitInterval {
		fmt.Print(".")
		time.Sleep(db.WaitInterval)

		// attempt connection to database server
		err = drv.Ping()
		if err == nil {
			// connection successful
			fmt.Print("\n")
			return nil
		}
	}

	// if we find outselves here, we could not connect within the timeout
	fmt.Print("\n")
	return fmt.Errorf("unable to connect to database: %s", err)
}

// CreateAndMigrate creates the database (if necessary) and runs migrations
func (db *DB) CreateAndMigrate() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
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
	return db.migrate(drv)
}

// Create creates the current database
func (db *DB) Create() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
	}

	return drv.CreateDatabase()
}

// Drop drops the current database (if it exists)
func (db *DB) Drop() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
	}

	return drv.DropDatabase()
}

// DumpSchema writes the current database schema to a file
func (db *DB) DumpSchema() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	return db.dumpSchema(drv)
}

func (db *DB) dumpSchema(drv Driver) error {
	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
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

	fmt.Printf("Writing: %s\n", db.SchemaFile)

	// ensure schema directory exists
	if err = ensureDir(filepath.Dir(db.SchemaFile)); err != nil {
		return err
	}

	// write schema to file
	return ioutil.WriteFile(db.SchemaFile, schema, 0644)
}

// ensureDir creates a directory if it does not already exist
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("unable to create directory `%s`", dir)
	}

	return nil
}

const migrationTemplate = "-- migrate:up\n\n\n-- migrate:down\n\n"

// NewMigration creates a new migration file
func (db *DB) NewMigration(name string) error {
	// new migration name
	timestamp := time.Now().UTC().Format("20060102150405")
	if name == "" {
		return fmt.Errorf("please specify a name for the new migration")
	}
	name = fmt.Sprintf("%s_%s.sql", timestamp, name)

	// create migrations dir if missing
	if err := ensureDir(db.MigrationsDir); err != nil {
		return err
	}

	// check file does not already exist
	path := filepath.Join(db.MigrationsDir, name)
	fmt.Printf("Creating migration: %s\n", path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("file already exists")
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
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	return db.migrate(drv)
}

func (db *DB) migrate(drv Driver) error {
	files, err := findMigrationFiles(db.MigrationsDir, migrationFileRegexp)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no migration files found")
	}

	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
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

		fmt.Printf("Applying: %s\n", filename)

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
				printVerbose(result)
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
		_ = db.dumpSchema(drv)
	}

	return nil
}

func printVerbose(result sql.Result) {
	lastInsertID, err := result.LastInsertId()
	if err == nil {
		fmt.Printf("Last insert ID: %d\n", lastInsertID)
	}
	rowsAffected, err := result.RowsAffected()
	if err == nil {
		fmt.Printf("Rows affected: %d\n", rowsAffected)
	}
}

func findMigrationFiles(dir string, re *regexp.Regexp) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not find migrations directory `%s`", dir)
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
		return "", fmt.Errorf("can't find migration file: %s*.sql", ver)
	}

	return files[0], nil
}

func migrationVersion(filename string) string {
	return regexp.MustCompile(`^\d+`).FindString(filename)
}

// Rollback rolls back the most recent migration
func (db *DB) Rollback() error {
	drv, err := db.GetDriver()
	if err != nil {
		return err
	}

	if db.WaitBefore {
		err := db.wait(drv)
		if err != nil {
			return err
		}
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
		return fmt.Errorf("can't rollback: no migrations have been applied")
	}

	filename, err := findMigrationFile(db.MigrationsDir, latest)
	if err != nil {
		return err
	}

	fmt.Printf("Rolling back: %s\n", filename)

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
			printVerbose(result)
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
		_ = db.dumpSchema(drv)
	}

	return nil
}

// Status shows the status of all migrations
func (db *DB) Status(quiet bool) (int, error) {
	drv, err := db.GetDriver()
	if err != nil {
		return -1, err
	}

	results, err := db.CheckMigrationsStatus(drv)
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
			fmt.Println(line)
		}
	}

	totalPending := len(results) - totalApplied
	if !quiet {
		fmt.Println()
		fmt.Printf("Applied: %d\n", totalApplied)
		fmt.Printf("Pending: %d\n", totalPending)
	}

	return totalPending, nil
}

// CheckMigrationsStatus returns the status of all available mgirations
func (db *DB) CheckMigrationsStatus(drv Driver) ([]StatusResult, error) {
	files, err := findMigrationFiles(db.MigrationsDir, migrationFileRegexp)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no migration files found")
	}

	sqlDB, err := db.openDatabaseForMigration(drv)
	if err != nil {
		return nil, err
	}
	defer dbutil.MustClose(sqlDB)

	applied, err := drv.SelectMigrations(sqlDB, -1)
	if err != nil {
		return nil, err
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
