# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Dbmate is a database migration tool written in Go that supports PostgreSQL, MySQL, SQLite, ClickHouse, BigQuery, and Spanner. It's a standalone CLI tool that uses plain SQL migration files with timestamp-based versioning. The tool can be used as both a CLI application and as a Go library.

## Development Commands

### Building
```bash
make build              # Build the dbmate binary to dist/dbmate
make build OUTPUT=name  # Build with custom output name
```

The build uses CGO and includes SQLite support. On Linux, binaries are statically linked for Alpine compatibility.

### Testing
```bash
make test              # Run all tests (requires database services running)
make docker-all        # Build Docker image and run full test suite
make docker-sh         # Start a development shell in Docker
```

Tests run with `-p 1` flag to ensure sequential execution across database drivers. Test databases are configured via environment variables:
- `POSTGRES_TEST_URL`
- `MYSQL_TEST_URL`
- `CLICKHOUSE_TEST_URL`
- `BIGQUERY_TEST_URL`
- `SPANNER_POSTGRES_TEST_URL`

### Running a Single Test
```bash
go test -v -p 1 -run TestMigrationParse ./pkg/dbmate/
go test -v -p 1 -run TestPostgres ./pkg/driver/postgres/
```

### Linting
```bash
make lint              # Run golangci-lint
make fix               # Run golangci-lint with auto-fix
```

### Waiting for Databases
```bash
make wait              # Wait for all test databases to become available
```

## Architecture

### Core Structure

The codebase is organized into three main packages under `pkg/`:

- **`pkg/dbmate`**: Core migration logic, CLI entry point, and driver interface
- **`pkg/dbutil`**: Database utilities and helper functions
- **`pkg/driver`**: Database-specific driver implementations (postgres, mysql, sqlite, clickhouse, bigquery)

### Driver Interface Pattern

Dbmate uses a plugin-style driver architecture. Each database driver implements the `Driver` interface defined in `pkg/dbmate/driver.go`:

```go
type Driver interface {
    Open() (*sql.DB, error)
    DatabaseExists() (bool, error)
    CreateDatabase() error
    DropDatabase() error
    DumpSchema(*sql.DB) ([]byte, error)
    MigrationsTableExists(*sql.DB) (bool, error)
    CreateMigrationsTable(*sql.DB) error
    SelectMigrations(*sql.DB, int) (map[string]bool, error)
    InsertMigration(dbutil.Transaction, string) error
    DeleteMigration(dbutil.Transaction, string) error
    Ping() error
    QueryError(string, error) error
}
```

Drivers self-register during initialization using `dbmate.RegisterDriver()` in their `init()` functions. For example, the Postgres driver registers itself for multiple URL schemes: `postgres`, `postgresql`, `redshift`, and `spanner-postgres`.

### Migration File Structure

Migration files use a specific comment-based syntax:

```sql
-- migrate:up
CREATE TABLE users (id serial);

-- migrate:down
DROP TABLE users;
```

**Important**: As of recent changes, migration files can contain **multiple migration sections**. Each section is a pair of `-- migrate:up` and `-- migrate:down` blocks. The entire file succeeds or fails as a unit, but each section is executed separately.

Migration options can be specified on the directive line:
```sql
-- migrate:up transaction:false
ALTER TYPE colors ADD VALUE 'orange';
```

### Migration Parsing

The migration parsing logic (`pkg/dbmate/migration.go`) handles:
- Finding all `-- migrate:up` directives in a file
- Pairing each with its corresponding `-- migrate:down` directive
- Extracting options like `transaction:false`
- Validating that no SQL statements appear before the first `-- migrate:up`

The parser returns a slice of `ParsedMigration` objects, one for each up/down section pair.

### Database Operations Flow

1. **Driver initialization**: `DB.Driver()` creates the appropriate driver based on URL scheme
2. **Wait handling**: If `WaitBefore` is set, the system waits for database connectivity
3. **Migration execution**:
   - `FindMigrations()` scans filesystem and queries the schema_migrations table
   - `Migrate()` applies pending migrations in order
   - Each migration section is executed (in transaction by default)
   - `InsertMigration()` records the version in schema_migrations
4. **Schema dumping**: After migrations, `DumpSchema()` is automatically called (unless `AutoDumpSchema` is false)

Schema dumps rely on external tools (`pg_dump`, `mysqldump`, `sqlite3`) being available in PATH.

### CLI Framework

The CLI uses `urfave/cli/v2`. The main application setup is in `main.go`:
- Global flags are defined at the app level (url, env, migrations-dir, etc.)
- Commands are registered with their specific flags and actions
- The `action()` wrapper function handles database initialization before each command

Environment variables are loaded from `.env` files using `joho/godotenv`. The loading happens before CLI parsing to support both command-line and environment-based configuration.

### Transaction Control

Migrations run inside transactions by default. Drivers that don't support transactional DDL (like ClickHouse and Spanner) should be used with `transaction:false`.

The `doTransaction()` function handles:
- Beginning a transaction
- Rolling back on error
- Committing on success

For `transaction:false` migrations, the raw `*sql.DB` is used directly as a `dbutil.Transaction`.

## Adding a New Database Driver

1. Create a new directory under `pkg/driver/`
2. Implement the `Driver` interface
3. Register the driver in an `init()` function with `dbmate.RegisterDriver()`
4. Import the driver in `main_cgo.go` (for SQLite) or `main.go`
5. Add test database service to `docker-compose.yml`
6. Update README.md with connection string format

## Key Files

- `main.go`: CLI application and command definitions
- `pkg/dbmate/db.go`: Core DB type and migration operations (Create, Migrate, Rollback, etc.)
- `pkg/dbmate/migration.go`: Migration file parsing logic
- `pkg/dbmate/driver.go`: Driver interface and registration
- `Makefile`: Build configuration with platform-specific settings

## Testing Notes

- Tests use real database instances via Docker Compose
- CGO is required for SQLite support (`export CGO_ENABLED=1`)
- Tests must run sequentially (`-p 1`) to avoid database conflicts
- Each driver has its own test file (e.g., `postgres_test.go`)
- ClickHouse has special cluster testing support with Zookeeper

## Special Considerations

### URL Parsing
Database URLs follow the format: `protocol://username:password@host:port/database_name?options`

Special parameters:
- PostgreSQL: `sslmode`, `search_path`, `socket`
- MySQL: `socket`
- SQLite: File paths (relative or absolute)
- ClickHouse: `on_cluster`, `cluster_macro`, `replica_macro`

### Strict Mode
When `--strict` is enabled, migrations must be applied in order. Out-of-order migrations cause failures.

### Schema Migrations Table
The `schema_migrations` table stores only the version string (extracted from filename). The filename format is `{version}_{description}.sql` where version is the leading numeric characters.
