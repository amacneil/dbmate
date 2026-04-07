# Set Role Validation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent `--set-role` from being used with non-transactional migrations where it cannot work reliably.

**Architecture:** Add upfront validation in `Migrate()` and `Rollback()` that checks pending migrations for `transaction:false` when `DatabaseRole` is set. Fail fast with clear error identifying incompatible migrations.

**Tech Stack:** Go, testify/require, fstest.MapFS for in-memory test migrations

---

### Task 1: Add error type and Migrate validation

**Files:**
- Modify: `pkg/dbmate/db.go:21-32` (add error to var block)
- Modify: `pkg/dbmate/db.go:381` (add validation after pending migrations collected)

**Step 1: Add the new error type**

In `pkg/dbmate/db.go`, add to the error var block (after line 31):

```go
ErrSetRoleWithNoTransaction = errors.New("--set-role cannot be used with non-transactional migrations")
```

**Step 2: Add import for strings package**

In `pkg/dbmate/db.go`, add `"strings"` to the import block if not present.

**Step 3: Add validation in Migrate()**

In `pkg/dbmate/db.go`, after line 380 (after the strict mode check, before `sqlDB, err := db.openDatabaseForMigration(drv)`), add:

```go
// Check for incompatible non-transactional migrations when using --set-role
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

**Step 4: Run linter**

Run: `golangci-lint run ./pkg/dbmate/...`
Expected: No errors related to db.go

**Step 5: Commit**

```bash
git add pkg/dbmate/db.go
git commit -m "Add validation to prevent --set-role with non-transactional migrations"
```

---

### Task 2: Add Rollback validation

**Files:**
- Modify: `pkg/dbmate/db.go:571` (add validation after parsing migration)

**Step 1: Add validation in Rollback()**

In `pkg/dbmate/db.go`, after the migration is parsed (after `parsedSections, err := latest.Parse()` and its error check, around line 571), add:

```go
// Check for incompatible non-transactional rollback when using --set-role
if db.DatabaseRole != nil {
	for _, section := range parsedSections {
		if !section.DownOptions.Transaction() {
			return fmt.Errorf("%w: %s", ErrSetRoleWithNoTransaction, latest.FileName)
		}
	}
}
```

**Step 2: Run linter**

Run: `golangci-lint run ./pkg/dbmate/...`
Expected: No errors

**Step 3: Commit**

```bash
git add pkg/dbmate/db.go
git commit -m "Add --set-role validation for rollback"
```

---

### Task 3: Write test for Migrate validation

**Files:**
- Modify: `pkg/dbmate/db_test.go` (add new test function)

**Step 1: Write the test**

Add after the existing `TestMigrateQueryErrorMessage` function (around line 800):

```go
func TestMigrateWithSetRoleAndNonTransactionalMigration(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))

	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	role := "test_role"
	db.DatabaseRole = &role

	db.FS = fstest.MapFS{
		"db/migrations/001_no_transaction.sql": {
			Data: []byte("-- migrate:up transaction:false\nCREATE TABLE test1 (id INT);\n-- migrate:down\nDROP TABLE test1;"),
		},
	}

	err = db.Migrate()
	require.Error(t, err)
	require.ErrorIs(t, err, dbmate.ErrSetRoleWithNoTransaction)
	require.Contains(t, err.Error(), "001_no_transaction.sql")
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./pkg/dbmate/... -run TestMigrateWithSetRoleAndNonTransactionalMigration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add pkg/dbmate/db_test.go
git commit -m "Add test for --set-role with non-transactional migration"
```

---

### Task 4: Write test for Rollback validation

**Files:**
- Modify: `pkg/dbmate/db_test.go` (add new test function)

**Step 1: Write the test**

Add after the previous test:

```go
func TestRollbackWithSetRoleAndNonTransactionalMigration(t *testing.T) {
	db := newTestDB(t, sqliteTestURL(t))

	err := db.Drop()
	require.NoError(t, err)
	err = db.Create()
	require.NoError(t, err)

	// First apply a migration without --set-role (transactional up, non-transactional down)
	db.FS = fstest.MapFS{
		"db/migrations/001_test.sql": {
			Data: []byte("-- migrate:up\nCREATE TABLE test1 (id INT);\n-- migrate:down transaction:false\nDROP TABLE test1;"),
		},
	}

	err = db.Migrate()
	require.NoError(t, err)

	// Now try to rollback with --set-role
	role := "test_role"
	db.DatabaseRole = &role

	err = db.Rollback()
	require.Error(t, err)
	require.ErrorIs(t, err, dbmate.ErrSetRoleWithNoTransaction)
	require.Contains(t, err.Error(), "001_test.sql")
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./pkg/dbmate/... -run TestRollbackWithSetRoleAndNonTransactionalMigration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add pkg/dbmate/db_test.go
git commit -m "Add test for --set-role with non-transactional rollback"
```

---

### Task 5: Run full test suite and verify

**Step 1: Run all tests**

Run: `go test ./pkg/dbmate/... -v`
Expected: All tests pass

**Step 2: Run linter on entire codebase**

Run: `golangci-lint run ./...`
Expected: No new errors

**Step 3: Final commit (if any cleanup needed)**

If any fixes were needed, commit them.
