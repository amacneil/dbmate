# Dbmate

[![Build Status](https://travis-ci.org/adrianmacneil/dbmate.svg?branch=master)](https://travis-ci.org/adrianmacneil/dbmate)

Dbmate is a database migration tool, to keep your database schema in sync across multiple developers and your production servers. It is a standalone command line tool, which can be used with any language or framework. This is especially helpful if you are writing many services in different languages, and want to maintain some sanity with consistent development tools.

## Features

* Supports PostgreSQL and MySQL.
* Powerful, [purpose-built DSL](https://en.wikipedia.org/wiki/SQL#Data_definition) for writing schema migrations.
* Migrations are timestamp-versioned, to avoid version number conflicts with multiple developers.
* Supports creating and dropping databases (handy in development/test).
* Database connection URL is definied using an environment variable (`DATABASE_URL` by default), or specified on the command line.
* Built-in support for reading environment variables from your `.env` file.
* Easy to distribute, single self-contained binary.

## Installation

Dbmate is currently under development. To install the latest build, run:

```sh
$ go get -u github.com/adrianmacneil/dbmate
```

## Commands

```sh
dbmate          # print help
dbmate new      # generate a new migration file
dbmate up       # create the database (if it does not already exist) and run any pending migrations
dbmate create   # create the database
dbmate drop     # drop the database
dbmate migrate  # run any pending migrations
dbmate rollback # roll back the most recent migration
```

## Usage

Dbmate locates your database using the `DATABASE_URL` environment variable by default. If you are writing a [twelve-factor app](http://12factor.net/), you should be storing all connection strings in environment variables.

To make this easy in development, dbmate looks for a `.env` file in the current directory, and treats any variables listed there as if they were specified in the current environment (existing environment variables take preference, however).

If you do not already have a `.env` file, create one and add your database connection URL:

```sh
$ cat .env
DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_development?sslmode=disable"
```

It is [generally recommended](https://github.com/bkeepers/dotenv#should-i-commit-my-env-file) to commit this file to source control, to make it easier for other developers to get up and running with your project. However, this is completely up to your personal preference.

> Note: When connecting to Postgres, you may need to add the `sslmode=disable` flag to your connection URL, as dbmate by default requires an SSL/TLS connection (some other frameworks/languages use unencrypted connections by default).

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
```

### Options

The following command line options are available with all commands. You must use command line arguments in the order `dbmate [global options] command [command options]`.

* `--migrations-dir, -d "./db/migrations"` - where to keep the migration files.
* `--env, -e "DATABASE_URL"` - specify an environment variable to read the database connection URL from.

For example, before running your test suite, you may wish to drop and recreate the test database. One easy way to do this is to store your test database connection URL in the `TEST_DATABASE_URL` environment variable:

```sh
$ cat .env
TEST_DATABASE_URL="postgres://postgres@127.0.0.1:5432/myapp_test?sslmode=disable"
```

You can then specify this environment variable in your test script (Makefile or similar):

```sh
$ dbmate -e TEST_DATABASE_URL drop
Dropping: myapp_test
$ dbmate -e TEST_DATABASE_URL up
Creating: myapp_test
Applying: 20151127184807_create_users_table.sql
```

## Contributing

Dbmate is written in Go, pull requests are welcome.

Tests are run against a real database using docker-compose. First, install the [Docker Toolbox](https://www.docker.com/docker-toolbox).

Make sure you have docker running:

```sh
$ docker-machine start default && eval "$(docker-machine env default)"
```

To build a docker image and run the tests:

```sh
$ make
```

To run just the lint and tests (without completely rebuilding the docker image):

```sh
$ make lint test
```
