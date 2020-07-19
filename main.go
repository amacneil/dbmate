package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"

	"github.com/joho/godotenv"
	"github.com/urfave/cli"

	"github.com/amacneil/dbmate/pkg/dbmate"
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

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "env, e",
			Value: "DATABASE_URL",
			Usage: "specify an environment variable containing the database URL",
		},
		cli.StringFlag{
			Name:  "migrations-dir, d",
			Value: dbmate.DefaultMigrationsDir,
			Usage: "specify the directory containing migration files",
		},
		cli.StringFlag{
			Name:  "schema-file, s",
			Value: dbmate.DefaultSchemaFile,
			Usage: "specify the schema file location",
		},
		cli.BoolFlag{
			Name:  "no-dump-schema",
			Usage: "don't update the schema file on migrate/rollback",
		},
		cli.BoolFlag{
			Name:  "wait",
			Usage: "wait for the db to become available before executing the subsequent command",
		},
		cli.DurationFlag{
			Name:  "wait-timeout",
			Usage: "timeout for --wait flag",
			Value: dbmate.DefaultWaitTimeout,
		},
	}

	app.Commands = []cli.Command{
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
				cli.BoolFlag{
					Name:  "verbose, v",
					Usage: "print the result of each statement execution",
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
				cli.BoolFlag{
					Name:  "verbose, v",
					Usage: "print the result of each statement execution",
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
				cli.BoolFlag{
					Name:  "verbose, v",
					Usage: "print the result of each statement execution",
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
				cli.BoolFlag{
					Name:  "exit-code",
					Usage: "return 1 if there are pending migrations",
				},
				cli.BoolFlag{
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
					return cli.NewExitError("", 1)
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
		db.AutoDumpSchema = !c.GlobalBool("no-dump-schema")
		db.MigrationsDir = c.GlobalString("migrations-dir")
		db.SchemaFile = c.GlobalString("schema-file")
		db.WaitBefore = c.GlobalBool("wait")
		overrideTimeout := c.GlobalDuration("wait-timeout")
		if overrideTimeout != 0 {
			db.WaitTimeout = overrideTimeout
		}

		return f(db, c)
	}
}

// getDatabaseURL returns the current environment database url
func getDatabaseURL(c *cli.Context) (u *url.URL, err error) {
	env := c.GlobalString("env")
	value := os.Getenv(env)

	return url.Parse(value)
}

// redactLogString attempts to redact passwords from errors
func redactLogString(in string) string {
	re := regexp.MustCompile("([a-zA-Z]+://[^:]+:)[^@]+@")

	return re.ReplaceAllString(in, "${1}********@")
}
