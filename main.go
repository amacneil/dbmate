package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	loadDotEnv()

	app := cli.NewApp()
	app.Name = "dbmate"
	app.Usage = "A lightweight, framework-independent database migration tool."

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "migrations-dir, d",
			Value: "./db/migrations",
			Usage: "specify the directory containing migration files",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "up",
			Usage: "Migrate to the latest version",
			Action: func(ctx *cli.Context) {
				runCommand(UpCommand, ctx)
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "pretend, p",
					Usage: "don't do anything; print migrations to apply",
				},
			},
		},
		{
			Name:  "new",
			Usage: "Generate a new migration file",
			Action: func(ctx *cli.Context) {
				runCommand(NewCommand, ctx)
			},
		},
		{
			Name:  "create",
			Usage: "Create database",
			Action: func(ctx *cli.Context) {
				runCommand(CreateCommand, ctx)
			},
		},
		{
			Name:  "drop",
			Usage: "Drop database (if it exists)",
			Action: func(ctx *cli.Context) {
				runCommand(DropCommand, ctx)
			},
		},
	}

	app.Run(os.Args)
}

type command func(*cli.Context) error

func runCommand(cmd command, ctx *cli.Context) {
	err := cmd(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func loadDotEnv() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
}
