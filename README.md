# Dbmate

[![Build Status](https://travis-ci.org/amacneil/dbmate.svg?branch=master)](https://travis-ci.org/amacneil/dbmate)
[![Go Report Card](https://goreportcard.com/badge/github.com/amacneil/dbmate)](https://goreportcard.com/report/github.com/amacneil/dbmate)
[![GitHub Release](https://img.shields.io/github/release/amacneil/dbmate.svg)](https://github.com/amacneil/dbmate/releases)

Dbmate is a database migration tool, to keep your database schema in sync across multiple developers and your production servers.

It is a standalone command line tool, which can be used with Go, Node.js, Python, Ruby, PHP, or any other language or framework you are using to write database-backed applications. This is especially helpful if you are writing many services in different languages, and want to maintain some sanity with consistent development tools.

For a comparison between dbmate and other popular database schema migration tools, please see the [Alternatives](#alternatives) table.

## Features

* Supports MySQL, PostgreSQL, and SQLite.
* Powerful, [purpose-built DSL](https://en.wikipedia.org/wiki/SQL#Data_definition) for writing schema migrations.
* Migrations are timestamp-versioned, to avoid version number conflicts with multiple developers.
* Migrations are run atomically inside a transaction.
* Supports creating and dropping databases (handy in development/test).
* Supports saving a `schema.sql` file to easily diff schema changes in git.
* Database connection URL is definied using an environment variable (`DATABASE_URL` by default), or specified on the command line.
* Built-in support for reading environment variables from your `.env` file.
* Easy to distribute, single self-contained binary.

## Installation

**OSX**

Install using Homebrew:

```sh
$ brew tap amacneil/dbmate
$ brew install dbmate
```

**Linux**

Download the binary directly:

```sh
$ sudo curl -fsSL -o /usr/local/bin/dbmate https://github.com/amacneil/dbmate/releases/download/v1.5.0/dbmate-linux-amd64
$ sudo chmod +x /usr/local/bin/dbmate
```

**Docker**

You can run dbmate using the official docker image:

```sh
$ docker run --rm amacneil/dbmate --help
```

**Heroku**

To use dbmate on Heroku, the easiest method is to store the linux binary in your git repository:

```sh
$ mkdir -p bin
$ curl -fsSL -o bin/dbmate-heroku https://github.com/amacneil/dbmate/releases/download/v1.5.0/dbmate-linux-amd64
$ chmod +x bin/dbmate-heroku
$ git add bin/dbmate-heroku
$ git commit -m "Add dbmate binary"
$ git push heroku master
```

You can then run dbmate on heroku:

```sh
$ heroku run bin/dbmate-heroku up
```

**Other**

Dbmate can be installed directly using `go get`:

```sh
$ go get -u github.com/amacneil/dbmate
```

## Commands

```sh
dbmate           # print help
dbmate new       # generate a new migration file
dbmate up        # create the database (if it does not already exist) and run any pending migrations
dbmate create    # create the database
dbmate drop      # drop the database
dbmate migrate   # run any pending migrations
dbmate rollback  # roll back the most recent migration
dbmate down      # alias for rollback
dbmate dump      # write the database schema.sql file
dbmate wait      # wait for the database server to become available
```

## Usage

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

* `protocol` must be one of `mysql`, `postgres`, `postgresql`, `sqlite`, `sqlite3`
* `host` can be either a hostname or IP address
* `options` are driver-specific (refer to the underlying Go SQL drivers if you wish to use these)

**MySQL**

```sh
DATABASE_URL="mysql://username:password@127.0.0.1:3306/database_name"
```

**PostgreSQL**

When connecting to Postgres, you may need to add the `sslmode=disable` option to your connection string, as dbmate by default requires a TLS connection (some other frameworks/languages allow unencrypted connections by default).

```sh
DATABASE_URL="postgres://username:password@127.0.0.1:5432/database_name?sslmode=disable"
```

**SQLite**

SQLite databases are stored on the filesystem, so you do not need to specify a host. By default, files are relative to the current directory. For example, the following will create a database at `./db/database_name.sqlite3`:

```sh
DATABASE_URL="sqlite:///db/database_name.sqlite3"
```

To specify an absolute path, add an additional forward slash to the path. The following will create a database at `/tmp/database_name.sqlite3`:

```sh
DATABASE_URL="sqlite:////tmp/database_name.sqlite3"
```

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

> Note: Migration files are named in the format `[version]_[description].sql`. Only the version (defined as all leading numeric characters in the file name) is recorded in the database, so you can safely rename a migration file without having any effect on its current application state.

### Running Migrations

Run `dbmate up` to run any pending migrations.

```sh
$ dbmate up
Creating: myapp_development
Applying: 20151127184807_create_users_table.sql
Writing: ./db/schema.sql
```

> Note: `dbmate up` will create the database if it does not already exist (assuming the current user has permission to create databases). If you want to run migrations without creating the database, run `dbmate migrate`.

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
Writing: ./db/schema.sql
```

### Migration Options

dbmate supports options passed to a migration block in the form of `key:value` pairs. List of supported options:

* `transaction`

#### transaction

`transaction` is useful if you need to run some SQL which cannot be executed from within a transaction. For example, in Postgres, you would need to disable transactions for migrations that alter an enum type to add a value:

```sql
-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange' AFTER 'red';
```

`transaction` will default to `true` if your database supports it.

### Schema File

When you run the `up`, `migrate`, or `rollback` commands, dbmate will automatically create a `./db/schema.sql` file containing a complete representation of your database schema. Dbmate keeps this file up to date for you, so you should not manually edit it.

It is recommended to check this file into source control, so that you can easily review changes to the schema in commits or pull requests. It's also possible to use this file when you want to quickly load a database schema, without running each migration sequentially (for example in your test harness). However, if you do not wish to save this file, you could add it to `.gitignore`, or pass the `--no-dump-schema` command line option.

To dump the `schema.sql` file without performing any other actions, run `dbmate dump`. Unlike other dbmate actions, this command relies on the respective `pg_dump`, `mysqldump`, or `sqlite3` commands being available in your PATH. If these tools are not available, dbmate will silenty skip the schema dump step during `up`, `migrate`, or `rollback` actions. You can diagnose the issue by running `dbmate dump` and looking at the output:

```sh
$ dbmate dump
exec: "pg_dump": executable file not found in $PATH
```

On Ubuntu or Debian systems, you can fix this by installing `postgresql-client`, `mysql-client`, or `sqlite3` respectively. Ensure that the package version you install is greater than or equal to the version running on your database server.

> Note: The `schema.sql` file will contain a complete schema for your database, even if some tables or columns were created outside of dbmate migrations.

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

You can chain `wait` together with other commands if you sometimes see failures caused by the database not yet being ready:

```sh
$ dbmate wait && dbmate up
Waiting for database....
Creating: myapp_development
```

If the database is still not available after 60 seconds, the command will return an error:

```sh
$ dbmate wait
Waiting for database............................................................
Error: unable to connect to database: pq: role "foobar" does not exist
```

Please note that the `wait` command does not verify whether your specified database exists, only that the server is available and ready (so it will return success if the database server is available, but your database has not yet been created).

### Options

The following command line options are available with all commands. You must use command line arguments in the order `dbmate [global options] command [command options]`.

* `--env, -e "DATABASE_URL"` - specify an environment variable to read the database connection URL from.
* `--migrations-dir, -d "./db/migrations"` - where to keep the migration files.
* `--schema-file, -s "./db/schema.sql"` - a path to keep the schema.sql file.
* `--no-dump-schema` - don't auto-update the schema.sql file on migrate/rollback

For example, before running your test suite, you may wish to drop and recreate the test database. One easy way to do this is to store your test database connection URL in the `TEST_DATABASE_URL` environment variable:

```sh
$ cat .env
TEST_DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_test?sslmode=disable"
```

You can then specify this environment variable in your test script (Makefile or similar):

```sh
$ dbmate -e TEST_DATABASE_URL drop
Dropping: myapp_test
$ dbmate -e TEST_DATABASE_URL --no-dump-schema up
Creating: myapp_test
Applying: 20151127184807_create_users_table.sql
```

## FAQ

**How do I use dbmate under Alpine linux?**

Alpine linux uses [musl libc](https://www.musl-libc.org/), which is incompatible with how we build SQLite support (using [cgo](https://golang.org/cmd/cgo/)). If you want Alpine linux support, and don't mind sacrificing SQLite support, please use the `dbmate-linux-musl-amd64` build found on the [releases page](https://github.com/amacneil/dbmate/releases).

## Alternatives

Why another database schema migration tool? Dbmate was inspired by many other tools, primarily [Active Record Migrations](http://guides.rubyonrails.org/active_record_migrations.html), with the goals of being trivial to configure, and language & framework independent. Here is a comparison between dbmate and other popular migration tools.

| | [goose](https://bitbucket.org/liamstask/goose/) | [sql-migrate](https://github.com/rubenv/sql-migrate) | [mattes/migrate](https://github.com/mattes/migrate) | [activerecord](http://guides.rubyonrails.org/active_record_migrations.html) | [sequelize](http://docs.sequelizejs.com/manual/tutorial/migrations.html) | [dbmate](https://github.com/amacneil/dbmate) |
| --- |:---:|:---:|:---:|:---:|:---:|:---:|
| **Features** |||||||
|Plain SQL migration files|:white_check_mark:|:white_check_mark:|:white_check_mark:|||:white_check_mark:|
|Support for creating and dropping databases||||:white_check_mark:||:white_check_mark:|
|Support for saving schema dump files||||:white_check_mark:||:white_check_mark:|
|Timestamp-versioned migration files|:white_check_mark:|||:white_check_mark:|:white_check_mark:|:white_check_mark:|
|Ability to wait for database to become ready||||||:white_check_mark:|
|Database connection string loaded from environment variables||||||:white_check_mark:|
|Automatically load .env file||||||:white_check_mark:|
|No separate configuration file||||:white_check_mark:|:white_check_mark:|:white_check_mark:|
|Language/framework independent|:eight_pointed_black_star:|:eight_pointed_black_star:|:eight_pointed_black_star:|||:white_check_mark:|
| **Drivers** |||||||
|PostgreSQL|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|
|MySQL|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|
|SQLite|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|:white_check_mark:|

> :eight_pointed_black_star: In theory these tools could be used with other languages, but a Go development environment is required because binary builds are not provided.

*If you notice any inaccuracies in this table, please [propose a change](https://github.com/amacneil/dbmate/edit/master/README.md).*

## Contributing

Dbmate is written in Go, pull requests are welcome.

Tests are run against a real database using docker-compose. To build a docker image and run the tests:

```sh
$ make docker
```

To start a development shell:

```sh
$ docker-compose run --rm dbmate bash
```
