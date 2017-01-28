package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/urfave/cli"
)

func main() {
	loadDotEnv()

	app := NewApp()
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// NewApp creates a new command line app
func NewApp() *cli.App {
	app := cli.NewApp()
	app.Name = "dbmate"
	app.Usage = "A lightweight, framework-independent database migration tool."
	app.Version = Version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "migrations-dir, d",
			Value: "./db/migrations",
			Usage: "specify the directory containing migration files",
		},
		cli.StringFlag{
			Name:  "env, e",
			Value: "DATABASE_URL",
			Usage: "specify an environment variable containing the database URL",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "new",
			Aliases: []string{"n"},
			Usage:   "Generate a new migration file",
			Action:  NewCommand,
		},
		{
			Name:   "up",
			Usage:  "Create database (if necessary) and migrate to the latest version",
			Action: UpCommand,
		},
		{
			Name:   "create",
			Usage:  "Create database",
			Action: CreateCommand,
		},
		{
			Name:   "drop",
			Usage:  "Drop database (if it exists)",
			Action: DropCommand,
		},
		{
			Name:   "migrate",
			Usage:  "Migrate to the latest version",
			Action: MigrateCommand,
		},
		{
			Name:    "rollback",
			Aliases: []string{"down"},
			Usage:   "Rollback the most recent migration",
			Action:  RollbackCommand,
		},
	}

	return app
}

type command func(*cli.Context) error

func loadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
}
