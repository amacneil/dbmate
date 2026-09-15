package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v2"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/bigquery"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/clickhouse"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/mysql"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

func main() {
	err := loadEnvFiles(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(3)
	}

	app := NewApp()
	err = app.Run(os.Args)

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
			Name:    "driver",
			EnvVars: []string{"DBMATE_DRIVER"},
			Usage:   "specify the driver to use (instead of deriving from the database URL scheme)",
		},
		&cli.StringFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Value:   "DATABASE_URL",
			Usage:   "specify an environment variable containing the database URL",
		},
		&cli.StringSliceFlag{
			Name:  "env-file",
			Value: cli.NewStringSlice(".env"),
			Usage: "specify a file to load environment variables from",
		},
		&cli.StringSliceFlag{
			Name:    "migrations-dir",
			Aliases: []string{"d"},
			EnvVars: []string{"DBMATE_MIGRATIONS_DIR"},
			Value:   cli.NewStringSlice(defaultDB.MigrationsDir[0]),
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
					Name:    "strict",
					EnvVars: []string{"DBMATE_STRICT"},
					Usage:   "fail if migrations would be applied out of order",
				},
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					EnvVars: []string{"DBMATE_VERBOSE"},
					Usage:   "print the result of each statement execution",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				db.Strict = c.Bool("strict")
				db.Verbose = c.Bool("verbose")
				return db.CreateAndMigrate()
			}),
		},
		{
			Name:  "create",
			Usage: "Create database",
			Action: action(func(db *dbmate.DB, _ *cli.Context) error {
				return db.Create()
			}),
		},
		{
			Name:  "drop",
			Usage: "Drop database (if it exists)",
			Action: action(func(db *dbmate.DB, _ *cli.Context) error {
				return db.Drop()
			}),
		},
		{
			Name:  "migrate",
			Usage: "Migrate to the latest version",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "strict",
					EnvVars: []string{"DBMATE_STRICT"},
					Usage:   "fail if migrations would be applied out of order",
				},
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					EnvVars: []string{"DBMATE_VERBOSE"},
					Usage:   "print the result of each statement execution",
				},
			},
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				db.Strict = c.Bool("strict")
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
				db.Strict = c.Bool("strict")
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
			Name: "dump",
			Usage: "Write the database schema to disk.\n" +
				"Supports passing extra arguments to the underlying dump tool for pg/mysql\n" +
				"example: dbmate dump -- --extra-flag",
			Action: action(func(db *dbmate.DB, c *cli.Context) error {
				// Capture all arguments provided after the `dump` command
				// and pass them through to the underlying tool
				// e.g. [--flag] for "dbmate dump -- --flag"
				db.Args = c.Args().Slice()
				return db.DumpSchema()
			}),
		},
		{
			Name:  "load",
			Usage: "Load schema file to the database",
			Action: action(func(db *dbmate.DB, _ *cli.Context) error {
				return db.LoadSchema()
			}),
		},
		{
			Name:  "wait",
			Usage: "Wait for the database to become available",
			Action: action(func(db *dbmate.DB, _ *cli.Context) error {
				return db.Wait()
			}),
		},
	}

	return app
}

// load environment variables from file(s)
func loadEnvFiles(args []string) error {
	var envFiles []string

	for i := 0; i < len(args); i++ {
		if args[i] == "--env-file" {
			if i+1 >= len(args) {
				// returning nil here, even though it's an error
				// because we want the caller to proceed anyway,
				// and produce the actual arg parsing error response
				return nil
			}

			envFiles = append(envFiles, args[i+1])
			i++
		}
	}

	if len(envFiles) == 0 {
		envFiles = []string{".env"}
	}

	// try to load all files in sequential order,
	// ignoring any that do not exist
	for _, file := range envFiles {
		err := godotenv.Load([]string{file}...)
		if err == nil {
			continue
		}

		var perr *os.PathError
		if errors.As(err, &perr) && errors.Is(perr, os.ErrNotExist) {
			// Ignoring file not found error
			continue
		}

		return fmt.Errorf("loading env file(s) %v: %v", envFiles, err)
	}

	return nil
}

// action wraps a cli.ActionFunc with dbmate initialization logic
func action(f func(*dbmate.DB, *cli.Context) error) cli.ActionFunc {
	return func(c *cli.Context) error {
		db, err := configureDB(c)
		if err != nil {
			return err
		}

		return f(db, c)
	}
}

func configureDB(c *cli.Context) (*dbmate.DB, error) {
	u, err := getDatabaseURL(c)
	if err != nil {
		return nil, err
	}

	db := dbmate.New(u)
	db.DriverName = c.String("driver")
	db.AutoDumpSchema = !c.Bool("no-dump-schema")
	db.MigrationsDir = c.StringSlice("migrations-dir")
	db.MigrationsTableName = c.String("migrations-table")
	db.SchemaFile = c.String("schema-file")
	db.WaitBefore = c.Bool("wait")
	waitTimeout := c.Duration("wait-timeout")
	if waitTimeout != 0 {
		db.WaitTimeout = waitTimeout
	}

	return db, nil
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
