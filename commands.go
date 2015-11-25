package main

import (
	"database/sql"
	"fmt"
	"github.com/adrianmacneil/dbmate/driver"
	"github.com/adrianmacneil/dbmate/driver/shared"
	"github.com/codegangsta/cli"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// CreateCommand creates the current database
func CreateCommand(ctx *cli.Context) error {
	u, err := GetDatabaseURL()
	if err != nil {
		return err
	}

	drv, err := driver.Get(u.Scheme)
	if err != nil {
		return err
	}

	return drv.CreateDatabase(u)
}

// DropCommand drops the current database (if it exists)
func DropCommand(ctx *cli.Context) error {
	u, err := GetDatabaseURL()
	if err != nil {
		return err
	}

	drv, err := driver.Get(u.Scheme)
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

	defer file.Close()
	_, err = file.WriteString(migrationTemplate)
	if err != nil {
		return err
	}

	return nil
}

// GetDatabaseURL returns the current environment database url
func GetDatabaseURL() (u *url.URL, err error) {
	return url.Parse(os.Getenv("DATABASE_URL"))
}

func doTransaction(db *sql.DB, txFunc func(shared.Transaction) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err := txFunc(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// MigrateCommand migrates database to the latest version
func MigrateCommand(ctx *cli.Context) error {
	migrationsDir := ctx.GlobalString("migrations-dir")
	available, err := findAvailableMigrations(migrationsDir)
	if err != nil {
		return err
	}

	if len(available) == 0 {
		return fmt.Errorf("No migration files found.")
	}

	u, err := GetDatabaseURL()
	if err != nil {
		return err
	}

	drv, err := driver.Get(u.Scheme)
	if err != nil {
		return err
	}

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := drv.CreateMigrationsTable(db); err != nil {
		return err
	}

	applied, err := drv.SelectMigrations(db)
	if err != nil {
		return err
	}

	for filename := range available {
		ver := migrationVersion(filename)
		if _, ok := applied[ver]; ok {
			// migration already applied
			continue
		}

		fmt.Printf("Applying: %s\n", filename)

		migration, err := parseMigration(filepath.Join(migrationsDir, filename))
		if err != nil {
			return err
		}

		// begin transaction
		doTransaction(db, func(tx shared.Transaction) error {
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

	}

	return nil
}

func findAvailableMigrations(dir string) (map[string]struct{}, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Could not find migrations directory `%s`.", dir)
	}

	nameRegexp := regexp.MustCompile(`^\d.*\.sql$`)
	migrations := map[string]struct{}{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !nameRegexp.MatchString(name) {
			continue
		}

		migrations[name] = struct{}{}
	}

	return migrations, nil
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
