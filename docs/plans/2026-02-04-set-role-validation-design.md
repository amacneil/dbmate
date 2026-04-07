# Set Role Validation for Non-Transactional Migrations

## Problem

The `--set-role` flag executes `SET ROLE` before migrations to run them under a specific PostgreSQL role. However, for non-transactional migrations (`-- migrate:up transaction:false`), `SET ROLE` is called on the `*sql.DB` connection pool, and subsequent SQL may execute on a different pooled connection. This means the role setting is lost and migrations run as the login user instead.

## Solution

Add validation to prevent `--set-role` from being used with non-transactional migrations. Abort with a clear error before running any migrations.

## Scope

**In scope:**
- Validation in `Migrate()` - check all pending migrations upfront
- Validation in `Rollback()` - check migration being rolled back
- Clear error messages identifying incompatible migrations

**Out of scope:**
- `dbmate load` command - not affected by `--set-role` (dev/test use case)
- Connection pinning for non-transactional migrations (YAGNI)

## Implementation

### New Error

```go
var ErrSetRoleWithNoTransaction = errors.New("--set-role cannot be used with non-transactional migrations")
```

### Migrate Validation

Location: `pkg/dbmate/db.go`, in `Migrate()` after finding pending migrations.

```go
if db.DatabaseRole != nil {
    var incompatible []string
    for _, migration := range pendingMigrations {
        parsed, err := migration.Parse()
        if err != nil {
            return err
        }
        for _, section := range parsed {
            if !section.UpOptions.Transaction() {
                incompatible = append(incompatible, migration.FileName)
                break
            }
        }
    }
    if len(incompatible) > 0 {
        return fmt.Errorf("%w: %s", ErrSetRoleWithNoTransaction, strings.Join(incompatible, ", "))
    }
}
```

### Rollback Validation

Location: `pkg/dbmate/db.go`, in `Rollback()` after parsing the migration.

```go
if db.DatabaseRole != nil {
    for _, section := range parsedSections {
        if !section.DownOptions.Transaction() {
            return fmt.Errorf("%w: %s", ErrSetRoleWithNoTransaction, latest.FileName)
        }
    }
}
```

### Error Message Format

Single migration:
```
dbmate: error: --set-role cannot be used with non-transactional migrations: 20240101120000_add_index.sql
```

Multiple migrations:
```
dbmate: error: --set-role cannot be used with non-transactional migrations: 20240101120000_add_index.sql, 20240102120000_another.sql
```

## Testing

### New Tests in `pkg/dbmate/db_test.go`

1. **`TestMigrateWithSetRoleAndNonTransactionalMigration`**
   - Set `db.DatabaseRole` to a test value
   - Create migration with `-- migrate:up transaction:false`
   - Assert `Migrate()` returns `ErrSetRoleWithNoTransaction`
   - Assert migration filename in error message

2. **`TestRollbackWithSetRoleAndNonTransactionalMigration`**
   - Set `db.DatabaseRole`
   - Apply migration with transactional up, non-transactional down
   - Assert `Rollback()` returns `ErrSetRoleWithNoTransaction`

3. **`TestMigrateWithSetRole`** (optional, skip if role setup is complex)
   - Verify normal transactional migrations work with `--set-role`

### Existing Coverage

PostgreSQL-specific tests in `pkg/driver/postgres/postgres_test.go` already verify `SET ROLE` functionality for transactional migrations.

## Files Changed

- `pkg/dbmate/db.go` - Add validation and error type
- `pkg/dbmate/db_test.go` - Add tests
