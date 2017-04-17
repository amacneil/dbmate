package main

import (
	"fmt"
	"log"
	"os"

	"github.com/flowhamster/dbmate/pkg/commands"
	"github.com/joho/godotenv"
	"github.com/urfave/cli"
)

func main() {
	loadDotEnv()

	app := commands.NewApp()
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
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
