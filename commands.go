package main

import (
	"database/sql"
	"fmt"
	"github.com/urfave/cli"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// UpCommand creates the database (if necessary) and runs migrations
func UpCommand(ctx *cli.Context) error {
	u, err := GetDatabaseURL(ctx)
	if err != nil {
		return err
	}

	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return err
	}

	// create database if it does not already exist
	// skip this step if we cannot determine status
	// (e.g. user does not have list database permission)
	exists, err := drv.DatabaseExists(u)
	if err == nil && !exists {
		if err := drv.CreateDatabase(u); err != nil {
			return err
		}
	}

	// migrate
	return MigrateCommand(ctx)
}

// CreateCommand creates the current database
func CreateCommand(ctx *cli.Context) error {
	u, err := GetDatabaseURL(ctx)
	if err != nil {
		return err
	}

	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return err
	}

	return drv.CreateDatabase(u)
}

// DropCommand drops the current database (if it exists)
func DropCommand(ctx *cli.Context) error {
	u, err := GetDatabaseURL(ctx)
	if err != nil {
		return err
	}

	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return err
	}

	return drv.DropDatabase(u)
}

const migrationTemplate = "-- migrate:up\n\n\n-- migrate:down\n\n"

// NewCommand creates a new migration file
func NewCommand(ctx *cli.Context) error {
	// new migration name
	timestamp := time.Now().UTC().Format("20060102150405")
	name := ctx.Args().First()
	if name == "" {
		return fmt.Errorf("Please specify a name for the new migration.")
	}
	name = fmt.Sprintf("%s_%s.sql", timestamp, name)

	// create migrations dir if missing
	migrationsDir := ctx.GlobalString("migrations-dir")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		return fmt.Errorf("Unable to create directory `%s`.", migrationsDir)
	}

	// check file does not already exist
	path := filepath.Join(migrationsDir, name)
	fmt.Printf("Creating migration: %s\n", path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("File already exists")
	}

	// write new migration
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer mustClose(file)
	_, err = file.WriteString(migrationTemplate)
	if err != nil {
		return err
	}

	return nil
}

// GetDatabaseURL returns the current environment database url
func GetDatabaseURL(ctx *cli.Context) (u *url.URL, err error) {
	env := ctx.GlobalString("env")
	value := os.Getenv(env)

	return url.Parse(value)
}

func doTransaction(db *sql.DB, txFunc func(Transaction) error) error {
	tx, err := db.Begin()
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

func openDatabaseForMigration(ctx *cli.Context) (Driver, *sql.DB, error) {
	u, err := GetDatabaseURL(ctx)
	if err != nil {
		return nil, nil, err
	}

	drv, err := GetDriver(u.Scheme)
	if err != nil {
		return nil, nil, err
	}

	db, err := drv.Open(u)
	if err != nil {
		return nil, nil, err
	}

	if err := drv.CreateMigrationsTable(db); err != nil {
		mustClose(db)
		return nil, nil, err
	}

	return drv, db, nil
}

// MigrateCommand migrates database to the latest version
func MigrateCommand(ctx *cli.Context) error {
	migrationsDir := ctx.GlobalString("migrations-dir")
	re := regexp.MustCompile(`^\d.*\.sql$`)
	files, err := findMigrationFiles(migrationsDir, re)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("No migration files found.")
	}

	drv, db, err := openDatabaseForMigration(ctx)
	if err != nil {
		return err
	}
	defer mustClose(db)

	applied, err := drv.SelectMigrations(db, -1)
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

		migration, err := parseMigration(filepath.Join(migrationsDir, filename))
		if err != nil {
			return err
		}

		// begin transaction
		err = doTransaction(db, func(tx Transaction) error {
			// run actual migration
			if _, err := tx.Exec(migration["up"]); err != nil {
				return err
			}

			// record migration
			if err := drv.InsertMigration(tx, ver); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}

	}

	return nil
}

func findMigrationFiles(dir string, re *regexp.Regexp) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Could not find migrations directory `%s`.", dir)
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
		return "", fmt.Errorf("Can't find migration file: %s*.sql", ver)
	}

	return files[0], nil
}

func migrationVersion(filename string) string {
	return regexp.MustCompile(`^\d+`).FindString(filename)
}

// parseMigration reads a migration file into a map with up/down keys
// implementation is similar to regexp.Split()
func parseMigration(path string) (map[string]string, error) {
	// read migration file into string
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	contents := string(data)

	// split string on our trigger comment
	separatorRegexp := regexp.MustCompile(`(?m)^-- migrate:(.*)$`)
	matches := separatorRegexp.FindAllStringSubmatchIndex(contents, -1)

	migrations := map[string]string{}
	direction := ""
	beg := 0
	end := 0

	for _, match := range matches {
		end = match[0]
		if direction != "" {
			// write previous direction to output map
			migrations[direction] = contents[beg:end]
		}

		// each match records the start of a new direction
		direction = contents[match[2]:match[3]]
		beg = match[1]
	}

	// write final direction to output map
	migrations[direction] = contents[beg:]

	return migrations, nil
}

// RollbackCommand rolls back the most recent migration
func RollbackCommand(ctx *cli.Context) error {
	drv, db, err := openDatabaseForMigration(ctx)
	if err != nil {
		return err
	}
	defer mustClose(db)

	applied, err := drv.SelectMigrations(db, 1)
	if err != nil {
		return err
	}

	// grab most recent applied migration (applied has len=1)
	latest := ""
	for ver := range applied {
		latest = ver
	}
	if latest == "" {
		return fmt.Errorf("Can't rollback: no migrations have been applied.")
	}

	migrationsDir := ctx.GlobalString("migrations-dir")
	filename, err := findMigrationFile(migrationsDir, latest)
	if err != nil {
		return err
	}

	fmt.Printf("Rolling back: %s\n", filename)

	migration, err := parseMigration(filepath.Join(migrationsDir, filename))
	if err != nil {
		return err
	}

	// begin transaction
	err = doTransaction(db, func(tx Transaction) error {
		// rollback migration
		if _, err := tx.Exec(migration["down"]); err != nil {
			return err
		}

		// remove migration record
		if err := drv.DeleteMigration(tx, latest); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
