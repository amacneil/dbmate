package dbmate

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/amacneil/dbmate/v2/pkg/dbutil"
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
var migrationFileRegexp = regexp.MustCompile(`^(\d+).*\.sql$`)

// DB allows dbmate actions to be performed on a specified database
type DB struct {
	// AutoDumpSchema generates schema.sql after each action
	AutoDumpSchema bool
	// DatabaseURL is the database connection string
	DatabaseURL *url.URL
	// DriverName used to force specific driver (overrides deriving from url scheme)
	DriverName string
	// FS specifies the filesystem, or nil for OS filesystem
	FS fs.FS
	// Log is the interface to write stdout
	Log io.Writer
	// MigrationsDir specifies the directory or directories to find migration files
	MigrationsDir []string
	// MigrationsTableName specifies the database table to record migrations in
	MigrationsTableName string
	// SchemaFile specifies the location for schema.sql file
	SchemaFile string
	// Fail if migrations would be applied out of order
	Strict bool
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
		FS:                  nil,
		Log:                 os.Stdout,
		MigrationsDir:       []string{"./db/migrations"},
		MigrationsTableName: "schema_migrations",
		SchemaFile:          "./db/schema.sql",
		Strict:              false,
		Verbose:             false,
		WaitBefore:          false,
		WaitInterval:        time.Second,
		WaitTimeout:         60 * time.Second,
	}
}

// Driver initializes the appropriate database driver
func (db *DB) Driver() (Driver, error) {
	if db.DatabaseURL == nil || db.DatabaseURL.Scheme == "" {
		return nil, ErrInvalidURL
	}

	driverName := db.DatabaseURL.Scheme
	if db.DriverName != "" {
		driverName = db.DriverName
	}
	driverFunc := drivers[driverName]
	if driverFunc == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, driverName)
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

// LoadSchema loads schema file to the current database
func (db *DB) LoadSchema() error {
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	sqlDB, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(sqlDB)

	_, err = os.Stat(db.SchemaFile)
	if err != nil {
		return err
	}

	fmt.Fprintf(db.Log, "Reading: %s\n", db.SchemaFile)

	bytes, err := os.ReadFile(db.SchemaFile)
	if err != nil {
		return err
	}

	// Strip psql meta-commands (e.g., \restrict, \unrestrict) that cannot be
	// executed directly against the database server.
	bytes, err = dbutil.StripPsqlMetaCommands(bytes)
	if err != nil {
		return err
	}

	result, err := sqlDB.Exec(string(bytes))
	if err != nil {
		return err
	} else if db.Verbose {
		db.printVerbose(result)
	}

	return nil
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
	if err := ensureDir(db.MigrationsDir[0]); err != nil {
		return err
	}

	// check file does not already exist
	path := filepath.Join(db.MigrationsDir[0], name)
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
	drv, err := db.Driver()
	if err != nil {
		return err
	}

	migrations, err := db.FindMigrations()
	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		return ErrNoMigrationFiles
	}

	highestAppliedMigrationVersion := ""
	pendingMigrations := []Migration{}
	for _, migration := range migrations {
		if migration.Applied {
			if db.Strict && highestAppliedMigrationVersion <= migration.Version {
				highestAppliedMigrationVersion = migration.Version
			}
		} else {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	if len(pendingMigrations) > 0 && db.Strict && pendingMigrations[0].Version <= highestAppliedMigrationVersion {
		return fmt.Errorf(
			"migration `%s` is out of order with already applied migrations, the version number has to be higher than the applied migration `%s` in --strict mode",
			pendingMigrations[0].Version,
			highestAppliedMigrationVersion,
		)
	}

	sqlDB, err := db.openDatabaseForMigration(drv)
	if err != nil {
		return err
	}
	defer dbutil.MustClose(sqlDB)

	for _, migration := range pendingMigrations {
		fmt.Fprintf(db.Log, "Applying: %s\n", migration.FileName)

		start := time.Now()

		parsed, err := migration.Parse()
		if err != nil {
			return err
		}

		for _, migrationSection := range parsed {
			execMigration := func(tx dbutil.Transaction) error {
				// run actual migration
				result, err := tx.Exec(migrationSection.Up)
				if err != nil {
					return drv.QueryError(migrationSection.Up, err)
				} else if db.Verbose {
					db.printVerbose(result)
				}

				// record migration
				return drv.InsertMigration(tx, migration.Version)
			}

			if migrationSection.UpOptions.Transaction() {
				// begin transaction
				err = doTransaction(sqlDB, execMigration)
			} else {
				// run outside of transaction
				err = execMigration(sqlDB)
			}

			elapsed := time.Since(start)
			fmt.Fprintf(db.Log, "Applied: %s in %s\n", migration.FileName, elapsed)

			if err != nil {
				return err
			}
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

func (db *DB) readMigrationsDir(dir string) ([]fs.DirEntry, error) {
	path := path.Clean(dir)

	// We use nil instead of os.DirFS() because DirFS cannot support both relative and absolute
	// directory paths - it must be anchored at either "." or "/", which we do not know in advance.
	// See: https://github.com/amacneil/dbmate/issues/403
	if db.FS == nil {
		return os.ReadDir(path)
	}

	return fs.ReadDir(db.FS, path)
}

// FindMigrations lists all available migrations
func (db *DB) FindMigrations() ([]Migration, error) {
	drv, err := db.Driver()
	if err != nil {
		return nil, err
	}

	sqlDB, err := drv.Open()
	if err != nil {
		return nil, err
	}
	defer dbutil.MustClose(sqlDB)

	// find applied migrations
	appliedMigrations := map[string]bool{}
	migrationsTableExists, err := drv.MigrationsTableExists(sqlDB)
	if err != nil {
		return nil, err
	}

	if migrationsTableExists {
		appliedMigrations, err = drv.SelectMigrations(sqlDB, -1)
		if err != nil {
			return nil, err
		}
	}

	migrations := []Migration{}
	for _, dir := range db.MigrationsDir {
		// find filesystem migrations
		files, err := db.readMigrationsDir(dir)
		if err != nil {
			return nil, fmt.Errorf("%w `%s`", ErrMigrationDirNotFound, dir)
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			matches := migrationFileRegexp.FindStringSubmatch(file.Name())
			if len(matches) < 2 {
				continue
			}

			migration := Migration{
				Applied:  false,
				FileName: matches[0],
				FilePath: path.Join(dir, matches[0]),
				FS:       db.FS,
				Version:  matches[1],
			}
			if ok := appliedMigrations[migration.Version]; ok {
				migration.Applied = true
			}

			migrations = append(migrations, migration)
		}
	}

	sort.Slice(
		migrations, func(i, j int) bool {
			return migrations[i].FileName < migrations[j].FileName
		},
	)

	return migrations, nil
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

	// find last applied migration
	var latest *Migration
	migrations, err := db.FindMigrations()
	if err != nil {
		return err
	}

	for i, migration := range migrations {
		if migration.Applied {
			latest = &migrations[i]
		}
	}

	if latest == nil {
		return ErrNoRollback
	}

	fmt.Fprintf(db.Log, "Rolling back: %s\n", latest.FileName)

	start := time.Now()

	parsedSections, err := latest.Parse()
	if err != nil {
		return err
	}

	for _, migrationSection := range parsedSections {
		execMigration := func(tx dbutil.Transaction) error {
			// rollback migration
			result, err := tx.Exec(migrationSection.Down)
			if err != nil {
				return drv.QueryError(migrationSection.Down, err)
			} else if db.Verbose {
				db.printVerbose(result)
			}

			// remove migration record
			return drv.DeleteMigration(tx, latest.Version)
		}

		if migrationSection.DownOptions.Transaction() {
			// begin transaction
			err = doTransaction(sqlDB, execMigration)
		} else {
			// run outside of transaction
			err = execMigration(sqlDB)
		}

		elapsed := time.Since(start)
		fmt.Fprintf(db.Log, "Rolled back: %s in %s\n", latest.FileName, elapsed)

		if err != nil {
			return err
		}

		// automatically update schema file, silence errors
		if db.AutoDumpSchema {
			_ = db.DumpSchema()
		}
	}

	return nil
}

// Status shows the status of all migrations
func (db *DB) Status(quiet bool) (int, error) {
	results, err := db.FindMigrations()
	if err != nil {
		return -1, err
	}

	var totalApplied int
	var line string

	for _, res := range results {
		if res.Applied {
			line = fmt.Sprintf("[X] %s", res.FileName)
			totalApplied++
		} else {
			line = fmt.Sprintf("[ ] %s", res.FileName)
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
