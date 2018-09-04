package main

import (
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/amacneil/dbmate/pkg/dbmate"
	"github.com/joho/godotenv"
	"github.com/urfave/cli"
)

func main() {
	loadDotEnv()

	app := NewApp()
	err := app.Run(os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
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
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
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
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.Migrate()
			}),
		},
		{
			Name:    "rollback",
			Aliases: []string{"down"},
			Usage:   "Rollback the most recent migration",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				return db.Rollback()
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
		log.Fatal("Error loading .env file")
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

		return f(db, c)
	}
}

// getDatabaseURL returns the current environment database url
func getDatabaseURL(c *cli.Context) (u *url.URL, err error) {
	env := c.GlobalString("env")
	value := os.Getenv(env)

	return url.Parse(value)
}
