package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v2"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/clickhouse"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/mysql"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

func main() {
	loadDotEnv()

	app := NewApp()
	err := app.Run(os.Args)

	if err != nil {
		errText := redactLogString(fmt.Sprintf("Error: %s\n", err))
		_, _ = fmt.Fprint(os.Stderr, errText)
		os.Exit(2)
	}
}

// NewApp creates a new command line app
func NewApp() *cli.App {
	app := cli.NewApp()
	app.Name = "dbmate"
	app.Usage = "A lightweight, framework-independent database migration tool."
	app.Version = dbmate.Version

	defaultDB := dbmate.New(nil)
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "url",
			Aliases: []string{"u"},
			Usage:   "specify the database URL",
		},
		&cli.StringFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Value:   "DATABASE_URL",
			Usage:   "specify an environment variable containing the database URL",
		},
		&cli.StringFlag{
			Name:    "migrations-dir",
			Aliases: []string{"d"},
			EnvVars: []string{"DBMATE_MIGRATIONS_DIR"},
			Value:   defaultDB.MigrationsDir,
			Usage:   "specify the directory containing migration files",
		},
		&cli.StringFlag{
			Name:    "migrations-table",
			EnvVars: []string{"DBMATE_MIGRATIONS_TABLE"},
			Value:   defaultDB.MigrationsTableName,
			Usage:   "specify the database table to record migrations in",
		},
		&cli.StringFlag{
			Name:    "schema-file",
			Aliases: []string{"s"},
			EnvVars: []string{"DBMATE_SCHEMA_FILE"},
			Value:   defaultDB.SchemaFile,
			Usage:   "specify the schema file location",
		},
		&cli.BoolFlag{
			Name:    "no-dump-schema",
			EnvVars: []string{"DBMATE_NO_DUMP_SCHEMA"},
			Usage:   "don't update the schema file on migrate/rollback",
		},
		&cli.BoolFlag{
			Name:    "wait",
			EnvVars: []string{"DBMATE_WAIT"},
			Usage:   "wait for the db to become available before executing the subsequent command",
		},
		&cli.DurationFlag{
			Name:    "wait-timeout",
			EnvVars: []string{"DBMATE_WAIT_TIMEOUT"},
			Usage:   "timeout for --wait flag",
			Value:   defaultDB.WaitTimeout,
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:    "new",
			Aliases: []string{"n"},
			Usage:   "Generate a new migration file",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				name := c.Args().First()
				return db.NewMigration(name)
			}),
		},
		{
			Name:  "up",
			Usage: "Create database (if necessary) and migrate to the latest version",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					EnvVars: []string{"DBMATE_VERBOSE"},
					Usage:   "print the result of each statement execution",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				db.Verbose = c.Bool("verbose")
				return db.CreateAndMigrate()
			}),
		},
		{
			Name:  "create",
			Usage: "Create database",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.Create()
			}),
		},
		{
			Name:  "drop",
			Usage: "Drop database (if it exists)",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.Drop()
			}),
		},
		{
			Name:  "migrate",
			Usage: "Migrate to the latest version",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					EnvVars: []string{"DBMATE_VERBOSE"},
					Usage:   "print the result of each statement execution",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				db.Verbose = c.Bool("verbose")
				return db.Migrate()
			}),
		},
		{
			Name:    "rollback",
			Aliases: []string{"down"},
			Usage:   "Rollback the most recent migration",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					EnvVars: []string{"DBMATE_VERBOSE"},
					Usage:   "print the result of each statement execution",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				db.Verbose = c.Bool("verbose")
				return db.Rollback()
			}),
		},
		{
			Name:  "status",
			Usage: "List applied and pending migrations",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "exit-code",
					Usage: "return 1 if there are pending migrations",
				},
				&cli.BoolFlag{
					Name:  "quiet",
					Usage: "don't output any text (implies --exit-code)",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				setExitCode := c.Bool("exit-code")
				quiet := c.Bool("quiet")
				if quiet {
					setExitCode = true
				}

				pending, err := db.Status(quiet)
				if err != nil {
					return err
				}

				if pending > 0 && setExitCode {
					return cli.Exit("", 1)
				}

				return nil
			}),
		},
		{
			Name:  "dump",
			Usage: "Write the database schema to disk",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.DumpSchema()
			}),
		},
		{
			Name:  "wait",
			Usage: "Wait for the database to become available",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.Wait()
			}),
		},
	}

	return app
}

// load environment variables from .env file
func loadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %s", err.Error())
	}
}

// action wraps a cli.ActionFunc with dbmate initialization logic
func action(f func(*dbmate.DB, *cli.Context) error) cli.ActionFunc {
	return func(c *cli.Context) error {
		u, err := getDatabaseURL(c)
		if err != nil {
			return err
		}
		db := dbmate.New(u)
		db.AutoDumpSchema = !c.Bool("no-dump-schema")
		db.MigrationsDir = c.String("migrations-dir")
		db.MigrationsTableName = c.String("migrations-table")
		db.SchemaFile = c.String("schema-file")
		db.WaitBefore = c.Bool("wait")
		waitTimeout := c.Duration("wait-timeout")
		if waitTimeout != 0 {
			db.WaitTimeout = waitTimeout
		}

		return f(db, c)
	}
}

// getDatabaseURL returns the current database url from cli flag or environment variable
func getDatabaseURL(c *cli.Context) (u *url.URL, err error) {
	// check --url flag first
	value := c.String("url")
	if value == "" {
		// if empty, default to --env or DATABASE_URL
		env := c.String("env")
		value = os.Getenv(env)
	}

	return url.Parse(value)
}

// redactLogString attempts to redact passwords from errors
func redactLogString(in string) string {
	re := regexp.MustCompile("([a-zA-Z]+://[^:]+:)[^@]+@")

	return re.ReplaceAllString(in, "${1}********@")
}
