# Dbmate

[![Release](https://img.shields.io/github/release/amacneil/dbmate.svg)](https://github.com/amacneil/dbmate/releases)
[![Go Report](https://goreportcard.com/badge/github.com/amacneil/dbmate)](https://goreportcard.com/report/github.com/amacneil/dbmate)
[![Reference](https://img.shields.io/badge/go.dev-reference-blue?logo=go&logoColor=white)](https://pkg.go.dev/github.com/amacneil/dbmate/v2/pkg/dbmate)

Dbmate is a database migration tool that will keep your database schema in sync across multiple developers and your production servers.

It is a standalone command line tool that can be used with Go, Node.js, Python, Ruby, PHP, Rust, C++, or any other language or framework you are using to write database-backed applications. This is especially helpful if you are writing multiple services in different languages, and want to maintain some sanity with consistent development tools.

For a comparison between dbmate and other popular database schema migration tools, please see [Alternatives](#alternatives).

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Commands](#commands)
  - [Command Line Options](#command-line-options)
- [Usage](#usage)
  - [Connecting to the Database](#connecting-to-the-database)
    - [PostgreSQL](#postgresql)
    - [MySQL](#mysql)
    - [SQLite](#sqlite)
    - [ClickHouse](#clickhouse)
    - [BigQuery](#bigquery)
    - [Spanner](#spanner)
  - [Creating Migrations](#creating-migrations)
  - [Running Migrations](#running-migrations)
  - [Rolling Back Migrations](#rolling-back-migrations)
  - [Migration Options](#migration-options)
  - [Waiting For The Database](#waiting-for-the-database)
  - [Exporting Schema File](#exporting-schema-file)
- [Library](#library)
  - [Use dbmate as a library](#use-dbmate-as-a-library)
  - [Embedding migrations](#embedding-migrations)
- [Concepts](#concepts)
  - [Migration files](#migration-files)
  - [Schema file](#schema-file)
  - [Schema migrations table](#schema-migrations-table)
- [Alternatives](#alternatives)
- [Contributing](#contributing)

## Features

- Supports MySQL, PostgreSQL, SQLite, and ClickHouse
- Uses plain SQL for writing schema migrations
- Migrations are timestamp-versioned, to avoid version number conflicts with multiple developers
- Migrations are run atomically inside a transaction
- Supports creating and dropping databases (handy in development/test)
- Supports saving a `schema.sql` file to easily diff schema changes in git
- Database connection URL is defined using an environment variable (`DATABASE_URL` by default), or specified on the command line
- Built-in support for reading environment variables from your `.env` file
- Easy to distribute, single self-contained binary
- Doesn't try to upsell you on a SaaS service

## Installation

**NPM**

Install using [NPM](https://www.npmjs.com/):

```sh
npm install --save-dev dbmate
npx dbmate --help
```

**macOS**

Install using [Homebrew](https://brew.sh/):

```sh
brew install dbmate
dbmate --help
```

**Linux**

Install the binary directly:

```sh
sudo curl -fsSL -o /usr/local/bin/dbmate https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-amd64
sudo chmod +x /usr/local/bin/dbmate
/usr/local/bin/dbmate --help
```

**Windows**

Install using [Scoop](https://scoop.sh)

```pwsh
scoop install dbmate
dbmate --help
```

**Docker**

Docker images are published to GitHub Container Registry ([`ghcr.io/amacneil/dbmate`](https://ghcr.io/amacneil/dbmate)).

Remember to set `--network=host` or see [this comment](https://github.com/amacneil/dbmate/issues/128#issuecomment-615924611) for more tips on using dbmate with docker networking):

```sh
docker run --rm -it --network=host ghcr.io/amacneil/dbmate --help
```

If you wish to create or apply migrations, you will need to use Docker's [bind mount](https://docs.docker.com/storage/bind-mounts/) feature to make your local working directory (`pwd`) available inside the dbmate container:

```sh
docker run --rm -it --network=host -v "$(pwd)/db:/db" ghcr.io/amacneil/dbmate new create_users_table
```

## Commands

```sh
dbmate --help    # print usage help
dbmate new       # generate a new migration file
dbmate up        # create the database (if it does not already exist) and run any pending migrations
dbmate create    # create the database
dbmate drop      # drop the database
dbmate migrate   # run any pending migrations
dbmate rollback  # roll back the most recent migration
dbmate down      # alias for rollback
dbmate status    # show the status of all migrations (supports --exit-code and --quiet)
dbmate dump      # write the database schema.sql file
dbmate load      # load schema.sql file to the database
dbmate wait      # wait for the database server to become available
```

### Command Line Options

The following options are available with all commands. You must use command line arguments in the order `dbmate [global options] command [command options]`. Most options can also be configured via environment variables (and loaded from your `.env` file, which is helpful to share configuration between team members).

- `--url, -u "protocol://host:port/dbname"` - specify the database url directly. _(env: `DATABASE_URL`)_
- `--driver "driver_name"` - specify the driver to use (if empty, the driver is derived from database URL scheme). _(env: `DBMATE_DRIVER`)_
- `--env, -e "DATABASE_URL"` - specify an environment variable to read the database connection URL from.
- `--env-file ".env"` - specify an alternate environment variables file(s) to load.
- `--migrations-dir, -d "./db/migrations"` - where to keep the migration files. _(env: `DBMATE_MIGRATIONS_DIR`)_
- `--migrations-table "schema_migrations"` - database table to record migrations in. _(env: `DBMATE_MIGRATIONS_TABLE`)_
- `--schema-file, -s "./db/schema.sql"` - a path to keep the schema.sql file. _(env: `DBMATE_SCHEMA_FILE`)_
- `--no-dump-schema` - don't auto-update the schema.sql file on migrate/rollback _(env: `DBMATE_NO_DUMP_SCHEMA`)_
- `--strict` - fail if migrations would be applied out of order _(env: `DBMATE_STRICT`)_
- `--wait` - wait for the db to become available before executing the subsequent command _(env: `DBMATE_WAIT`)_
- `--wait-timeout 60s` - timeout for --wait flag _(env: `DBMATE_WAIT_TIMEOUT`)_

## Usage

### Connecting to the Database

Dbmate locates your database using the `DATABASE_URL` environment variable by default. If you are writing a [twelve-factor app](http://12factor.net/), you should be storing all connection strings in environment variables.

To make this easy in development, dbmate looks for a `.env` file in the current directory, and treats any variables listed there as if they were specified in the current environment (existing environment variables take preference, however).

If you do not already have a `.env` file, create one and add your database connection URL:

```sh
$ cat .env
DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_development?sslmode=disable"
```

`DATABASE_URL` should be specified in the following format:

```
protocol://username:password@host:port/database_name?options
```

- `protocol` must be one of `mysql`, `postgres`, `postgresql`, `sqlite`, `sqlite3`, `clickhouse`
- `username` and `password` must be URL encoded (you will get an error if you use special charactors)
- `host` can be either a hostname or IP address
- `options` are driver-specific (refer to the underlying Go SQL drivers if you wish to use these)

Dbmate can also load the connection URL from a different environment variable. For example, before running your test suite, you may wish to drop and recreate the test database. One easy way to do this is to store your test database connection URL in the `TEST_DATABASE_URL` environment variable:

```sh
$ cat .env
DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_dev?sslmode=disable"
TEST_DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_test?sslmode=disable"
```

You can then specify this environment variable in your test script (Makefile or similar):

```sh
$ dbmate -e TEST_DATABASE_URL drop
Dropping: myapp_test
$ dbmate -e TEST_DATABASE_URL --no-dump-schema up
Creating: myapp_test
Applying: 20151127184807_create_users_table.sql
Applied: 20151127184807_create_users_table.sql in 123µs
```

Alternatively, you can specify the url directly on the command line:

```sh
$ dbmate -u "postgres://postgres@127.0.0.1:5432/myapp_test?sslmode=disable" up
```

The only advantage of using `dbmate -e TEST_DATABASE_URL` over `dbmate -u $TEST_DATABASE_URL` is that the former takes advantage of dbmate's automatic `.env` file loading.

#### PostgreSQL

When connecting to Postgres, you may need to add the `sslmode=disable` option to your connection string, as dbmate by default requires a TLS connection (some other frameworks/languages allow unencrypted connections by default).

```sh
DATABASE_URL="postgres://username:password@127.0.0.1:5432/database_name?sslmode=disable"
```

A `socket` or `host` parameter can be specified to connect through a unix socket (note: specify the directory only):

```sh
DATABASE_URL="postgres://username:password@/database_name?socket=/var/run/postgresql"
```

A `search_path` parameter can be used to specify the [current schema](https://www.postgresql.org/docs/13/ddl-schemas.html#DDL-SCHEMAS-PATH) while applying migrations, as well as for dbmate's `schema_migrations` table.
If the schema does not exist, it will be created automatically. If multiple comma-separated schemas are passed, the first will be used for the `schema_migrations` table.

```sh
DATABASE_URL="postgres://username:password@127.0.0.1:5432/database_name?search_path=myschema"
```

```sh
DATABASE_URL="postgres://username:password@127.0.0.1:5432/database_name?search_path=myschema,public"
```

#### MySQL

```sh
DATABASE_URL="mysql://username:password@127.0.0.1:3306/database_name"
```

A `socket` parameter can be specified to connect through a unix socket:

```sh
DATABASE_URL="mysql://username:password@/database_name?socket=/var/run/mysqld/mysqld.sock"
```

#### SQLite

SQLite databases are stored on the filesystem, so you do not need to specify a host. By default, files are relative to the current directory. For example, the following will create a database at `./db/database.sqlite3`:

```sh
DATABASE_URL="sqlite:db/database.sqlite3"
```

To specify an absolute path, add a forward slash to the path. The following will create a database at `/tmp/database.sqlite3`:

```sh
DATABASE_URL="sqlite:/tmp/database.sqlite3"
```

Note that for some common [settings](https://sqlite.org/pragma.html) like `journal_mode` to improve performance, transactions need to be disabled for that migration file, e.g.

```sql
-- migrate:up transaction:false
PRAGMA journal_mode = WAL;
```

Otherwise the migration will fail with "Error: cannot change into wal mode from within a transaction".

#### ClickHouse

Dbmate supports connecting to ClickHouse using native TCP (default) or HTTP/HTTPS.

##### Native (TCP)

By default, the `clickhouse://` scheme uses the native protocol on port `9000`.

```sh
DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name"
```

##### HTTP / HTTPS

You can use `clickhouse+http://` (deafult port 8123) or `clickhouse+https://` (default port 8443).

```sh
# HTTP (Defaults to port 8123)
DATABASE_URL="clickhouse+http://username:password@127.0.0.1:8123/database_name"

# HTTPS (Defaults to port 8443)
DATABASE_URL="clickhouse+https://username:password@127.0.0.1:8443/database_name"
```

##### Using the --driver flag

You can use the ClickHouse driver with a standard http/https/tcp URL by providing the --driver flag

```sh
# Connect via HTTP using generic URL syntax
dbmate --driver clickhouse --url "http://username:password@127.0.0.1:8123/database_name" status

dbmate --driver clickhouse --url "https://username:password@127.0.0.1:8443/database_name" status

# Better to rely on the standard clickhouse:// scheme, but this is supported
dbmate --driver clickhouse --url "tcp://username:password@127.0.0.1:9000/database_name" status
```

To work with ClickHouse cluster, there are 4 connection query parameters that can be supplied:

- `on_cluster` - Indicataion to use cluster statements and replicated migration table. (default: `false`) If this parameter is not supplied, other cluster related query parameters are ignored.

```sh
DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name?on_cluster"

DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name?on_cluster=true"
```

- `cluster_macro` (Optional) - Macro value to be used for ON CLUSTER statements and for the replciated migration table engine zookeeper path. (default: `{cluster}`)

```sh
DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name?on_cluster&cluster_macro={my_cluster}"
```

- `replica_macro` (Optional) - Macro value to be used for the replica name in the replciated migration table engine. (default: `{replica}`)

```sh
DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name?on_cluster&replica_macro={my_replica}"
```

- `zoo_path` (Optional) - The path to the table migration in ClickHouse/Zoo Keeper. (default: `/clickhouse/tables/<cluster_macro>/{table}`)

```sh
DATABASE_URL="clickhouse://username:password@127.0.0.1:9000/database_name?on_cluster&zoo_path=/zk/path/tables"
```

[See other supported connection options](https://github.com/ClickHouse/clickhouse-go#dsn).

#### BigQuery

Follow the following format for `DATABASE_URL` when connecting to actual BigQuery in GCP:

```
bigquery://projectid/location/dataset
```

`projectid` (mandatory) - Project ID

`dataset` (mandatory) - Dataset name within the Project

`location` (optional) - Where Dataset is created

_NOTE: Follow [this doc](https://cloud.google.com/docs/authentication/provide-credentials-adc) on how to set `GOOGLE_APPLICATION_CREDENTIALS` environment variable for proper Authentication_

Follow the following format if trying to connect to a custom endpoint e.g. [BigQuery Emulator](https://github.com/goccy/bigquery-emulator)

```
bigquery://host:port/projectid/location/dataset?disable_auth=true
```

`disable_auth` (optional) - Pass `true` to skip Authentication, use only for testing and connecting to emulator.

#### Spanner

Spanner support is currently limited to databases using the [PostgreSQL Dialect](https://cloud.google.com/spanner/docs/postgresql-interface), which must be chosen during database creation. For future Spanner with GoogleSQL support, see [this discussion](https://github.com/amacneil/dbmate/discussions/369).

Spanner with the Postgres interface requires that the [PGAdapter](https://cloud.google.com/spanner/docs/pgadapter) is running. Use the following format for `DATABASE_URL`, with the host and port set to where the PGAdapter is running:

```shell
DATABASE_URL="spanner-postgres://127.0.0.1:5432/database_name?sslmode=disable"
```

Note that specifying a username and password is not necessary, as authentication is handled by the PGAdapter (they will be ignored by the PGAdapter if specified).

Other options of the [postgres driver](#postgresql) are supported.

Spanner also doesn't allow DDL to be executed inside explicit transactions. You must therefore specify `transaction:false` on migrations that include DDL:

```sql
-- migrate:up transaction:false
CREATE TABLE ...

-- migrate:down transaction:false
DROP TABLE ...
```

Schema dumps are not currently supported, as `pg_dump` uses functions that are not provided by Spanner.

### Creating Migrations

To create a new migration, run `dbmate new create_users_table`. You can name the migration anything you like. This will create a file `db/migrations/20151127184807_create_users_table.sql` in the current directory:

```sql
-- migrate:up

-- migrate:down
```

To write a migration, simply add your SQL to the `migrate:up` section:

```sql
-- migrate:up
create table users (
  id integer,
  name varchar(255),
  email varchar(255) not null
);

-- migrate:down
```

For related changes, it is possible to include multiple migrations in a single file using additional `migrate:up` and `migrate:down` sections. Migration file either succeeds or fails as a whole.

```sql
-- migrate:up
CREATE TABLE users (id SERIAL PRIMARY KEY);

-- migrate:down
DROP TABLE users;

-- migrate:up
ALTER TABLE users ADD COLUMN email VARCHAR;

-- migrate:down
ALTER TABLE users DROP COLUMN email;
```

> Note: Migration files are named in the format `[version]_[description].sql`. Only the version (defined as all leading numeric characters in the file name) is recorded in the database, so you can safely rename a migration file without having any effect on its current application state.

### Running Migrations

Run `dbmate up` to run any pending migrations.

```sh
$ dbmate up
Creating: myapp_development
Applying: 20151127184807_create_users_table.sql
Applied: 20151127184807_create_users_table.sql in 123µs
Writing: ./db/schema.sql
```

> Note: `dbmate up` will create the database if it does not already exist (assuming the current user has permission to create databases). If you want to run migrations without creating the database, run `dbmate migrate`.

Pending migrations are always applied in numerical order. However, dbmate does not prevent migrations from being applied out of order if they are committed independently (for example: if a developer has been working on a branch for a long time, and commits a migration which has a lower version number than other already-applied migrations, dbmate will simply apply the pending migration). See [#159](https://github.com/amacneil/dbmate/issues/159) for a more detailed explanation.

### Rolling Back Migrations

By default, dbmate doesn't know how to roll back a migration. In development, it's often useful to be able to revert your database to a previous state. To accomplish this, implement the `migrate:down` section:

```sql
-- migrate:up
create table users (
  id integer,
  name varchar(255),
  email varchar(255) not null
);

-- migrate:down
drop table users;
```

Run `dbmate rollback` to roll back the most recent migration:

```sh
$ dbmate rollback
Rolling back: 20151127184807_create_users_table.sql
Rolled back: 20151127184807_create_users_table.sql in 123µs
Writing: ./db/schema.sql
```

### Migration Options

dbmate supports options passed to a migration block in the form of `key:value` pairs. List of supported options:

- `transaction`

**transaction**

`transaction` is useful if you do not want to run SQL inside a transaction:

```sql
-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
```

`transaction` will default to `true` if your database supports it.

### Waiting For The Database

If you use a Docker development environment for your project, you may encounter issues with the database not being immediately ready when running migrations or unit tests. This can be due to the database server having only just started.

In general, your application should be resilient to not having a working database connection on startup. However, for the purpose of running migrations or unit tests, this is not practical. The `wait` command avoids this situation by allowing you to pause a script or other application until the database is available. Dbmate will attempt a connection to the database server every second, up to a maximum of 60 seconds.

If the database is available, `wait` will return no output:

```sh
$ dbmate wait
```

If the database is unavailable, `wait` will block until the database becomes available:

```sh
$ dbmate wait
Waiting for database....
```

You can also use the `--wait` flag with other commands if you sometimes see failures caused by the database not yet being ready:

```sh
$ dbmate --wait up
Waiting for database....
Creating: myapp_development
```

You can customize the timeout using `--wait-timeout` (default 60s). If the database is still not available, the command will return an error:

```sh
$ dbmate --wait-timeout=5s wait
Waiting for database.....
Error: unable to connect to database: dial tcp 127.0.0.1:5432: connect: connection refused
```

Please note that the `wait` command does not verify whether your specified database exists, only that the server is available and ready (so it will return success if the database server is available, but your database has not yet been created).

### Exporting Schema File

When you run the `up`, `migrate`, or `rollback` commands, dbmate will automatically create a `./db/schema.sql` file containing a complete representation of your database schema. Dbmate keeps this file up to date for you, so you should not manually edit it.

It is recommended to check this file into source control, so that you can easily review changes to the schema in commits or pull requests. It's also possible to use this file when you want to quickly load a database schema, without running each migration sequentially (for example in your test harness). However, if you do not wish to save this file, you could add it to your `.gitignore`, or pass the `--no-dump-schema` command line option.

To dump the `schema.sql` file without performing any other actions, run `dbmate dump`. Unlike other dbmate actions, this command relies on the respective `pg_dump`, `mysqldump`, or `sqlite3` commands being available in your PATH. If these tools are not available, dbmate will silently skip the schema dump step during `up`, `migrate`, or `rollback` actions. You can diagnose the issue by running `dbmate dump` and looking at the output:

```sh
$ dbmate dump
exec: "pg_dump": executable file not found in $PATH
```

On Ubuntu or Debian systems, you can fix this by installing `postgresql-client`, `mysql-client`, or `sqlite3` respectively. Ensure that the package version you install is greater than or equal to the version running on your database server.

> Note: The `schema.sql` file will contain a complete schema for your database, even if some tables or columns were created outside of dbmate migrations.

## Library

### Use dbmate as a library

Dbmate is designed to be used as a CLI with any language or framework, but it can also be used as a library in a Go application.

Here is a simple example. Remember to import the driver you need!

```go
package main

import (
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
)

func main() {
	u, _ := url.Parse("sqlite:foo.sqlite3")
	db := dbmate.New(u)

	err := db.CreateAndMigrate()
	if err != nil {
		panic(err)
	}
}
```

See the [reference documentation](https://pkg.go.dev/github.com/amacneil/dbmate/v2/pkg/dbmate) for more options.

### Embedding migrations

Migrations can be embedded into your application binary using Go's [embed](https://pkg.go.dev/embed) functionality.

Use `db.FS` to specify the filesystem used for reading migrations:

```go
package main

import (
	"embed"
	"fmt"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite"
)

//go:embed db/migrations/*.sql
var fs embed.FS

func main() {
	u, _ := url.Parse("sqlite:foo.sqlite3")
	db := dbmate.New(u)
	db.FS = fs

	fmt.Println("Migrations:")
	migrations, err := db.FindMigrations()
	if err != nil {
		panic(err)
	}
	for _, m := range migrations {
		fmt.Println(m.Version, m.FilePath)
	}

	fmt.Println("\nApplying...")
	err = db.CreateAndMigrate()
	if err != nil {
		panic(err)
	}
}
```

## Concepts

### Migration files

Migration files are very simple, and are stored in `./db/migrations` by default. You can create a new migration file named `[date]_create_users.sql` by running `dbmate new create_users`.
Here is an example:

```sql
-- migrate:up
create table users (
  id integer,
  name varchar(255),
);

-- migrate:down
drop table if exists users;
```

Both up and down migrations are stored in the same file, for ease of editing. Both up and down directives are required, even if you choose not to implement the down migration.

When you apply a migration dbmate only stores the version number, not the contents, so you should always rollback a migration before modifying its contents. For this reason, you can safely rename a migration file without affecting its applied status, as long as you keep the version number intact.

### Schema file

The schema file is written to `./db/schema.sql` by default. It is a complete dump of your database schema, including any applied migrations, and any other modifications you have made.

This file should be checked in to source control, so that you can easily compare the diff of a migration. You can use the schema file to quickly restore your database without needing to run all migrations.

### Schema migrations table

Dbmate stores a record of each applied migration in table named `schema_migrations`. This table will be created for you automatically if it does not already exist.

The table is very simple:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) PRIMARY KEY
)
```

You can customize the name of this table using the `--migrations-table` flag or `DBMATE_MIGRATIONS_TABLE` environment variable.

## Alternatives

Why another database schema migration tool? Dbmate was inspired by many other tools, primarily [Active Record Migrations](http://guides.rubyonrails.org/active_record_migrations.html), with the goals of being trivial to configure, and language & framework independent. Here is a comparison between dbmate and other popular migration tools.

|                                                              | [dbmate](https://github.com/amacneil/dbmate) | [goose](https://github.com/pressly/goose) | [sql-migrate](https://github.com/rubenv/sql-migrate) | [golang-migrate](https://github.com/golang-migrate/migrate) | [activerecord](http://guides.rubyonrails.org/active_record_migrations.html) | [sequelize](http://docs.sequelizejs.com/manual/tutorial/migrations.html) | [flyway](https://flywaydb.org/) | [sqitch](https://sqitch.org/) |
| ------------------------------------------------------------ | :------------------------------------------: | :---------------------------------------: | :--------------------------------------------------: | :---------------------------------------------------------: | :-------------------------------------------------------------------------: | :----------------------------------------------------------------------: | :-----------------------------: | :---------------------------: |
| **Features**                                                 |
| Plain SQL migration files                                    |              :white_check_mark:              |            :white_check_mark:             |                  :white_check_mark:                  |                     :white_check_mark:                      |                                                                             |                                                                          |       :white_check_mark:        |      :white_check_mark:       |
| Support for creating and dropping databases                  |              :white_check_mark:              |                                           |                                                      |                                                             |                             :white_check_mark:                              |                                                                          |                                 |
| Support for saving schema dump files                         |              :white_check_mark:              |                                           |                                                      |                                                             |                             :white_check_mark:                              |                                                                          |                                 |
| Timestamp-versioned migration files                          |              :white_check_mark:              |            :white_check_mark:             |                                                      |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |                                 |
| Custom schema migrations table                               |              :white_check_mark:              |                                           |                  :white_check_mark:                  |                                                             |                                                                             |                            :white_check_mark:                            |       :white_check_mark:        |
| Ability to wait for database to become ready                 |              :white_check_mark:              |                                           |                                                      |                                                             |                                                                             |                                                                          |                                 |
| Database connection string loaded from environment variables |              :white_check_mark:              |                                           |                                                      |                                                             |                                                                             |                                                                          |       :white_check_mark:        |
| Automatically load .env file                                 |              :white_check_mark:              |                                           |                                                      |                                                             |                                                                             |                                                                          |                                 |
| No separate configuration file                               |              :white_check_mark:              |            :white_check_mark:             |                                                      |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |                                 |
| Language/framework independent                               |              :white_check_mark:              |            :white_check_mark:             |                                                      |                     :white_check_mark:                      |                                                                             |                                                                          |       :white_check_mark:        |      :white_check_mark:       |
| **Drivers**                                                  |
| PostgreSQL                                                   |              :white_check_mark:              |            :white_check_mark:             |                  :white_check_mark:                  |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |       :white_check_mark:        |      :white_check_mark:       |
| MySQL                                                        |              :white_check_mark:              |            :white_check_mark:             |                  :white_check_mark:                  |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |       :white_check_mark:        |      :white_check_mark:       |
| SQLite                                                       |              :white_check_mark:              |            :white_check_mark:             |                  :white_check_mark:                  |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |       :white_check_mark:        |      :white_check_mark:       |
| CliсkHouse                                                   |              :white_check_mark:              |                                           |                                                      |                     :white_check_mark:                      |                             :white_check_mark:                              |                            :white_check_mark:                            |                                 |

_If you notice any inaccuracies in this table, please [propose a change](https://github.com/amacneil/dbmate/edit/main/README.md)._

## Contributing

Dbmate is written in Go, pull requests are welcome.

Tests are run against a real database using docker compose. To build a docker image and run the tests:

```sh
$ make docker-all
```

To start a development shell:

```sh
$ make docker-sh
```
