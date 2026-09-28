# DM8 Case-Sensitive SQL Audit Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task by task. Steps use checkbox syntax for tracking.

**Goal:** Make application SQL paths work with a `CASE_SENSITIVE=1` DM8 schema created by GORM with quoted lowercase identifiers.

**Architecture:** Keep identifiers under application control and quote them through GORM's active dialect. Leave bound values as parameters. Audit raw SQL expressions by business area, then verify each area against a real isolated DM8 schema before running the ordinary suite.

**Tech Stack:** Go, GORM, DM8, SQLite, Go integration tests.

**Spec:** [DM8 case-sensitive verification](../../dm8-case-sensitive.md).

## Global Constraints

- Never run `TestDM8StartupMigration` on the schema used for manual validation; it drops all tables.
- Only use test schemas with prefix `NEWAPI_TEST_` and verify `CASE_SENSITIVE()=1`.
- Keep MySQL, PostgreSQL, and SQLite SQL generation valid.
- Preserve existing uncommitted edits and the user's running Docker container.

## Review Focus

- Aggregate aliases must scan into Go structs.
- Quoted group and order expressions must work on DM8 and SQLite.
- Conflict updates must add to existing counters rather than replace them.
- Deletes must stay within the intended time or owner filter.
- Test data must be rolled back or cleaned without clearing a shared schema.

### Task 1: Quota data and flow dashboard

**Files:** `model/usedata.go`, `model/usedata_flow.go`, `model/dm8_integration_test.go`.

- [x] Add a real DM8 test for the reported `GetAllQuotaDates` query and related quota summaries; verify it fails with `-2111`.
- [x] Quote all identifiers in quota data aggregate, filter, group, order, and update expressions.
- [x] Re-run the focused test and compare expected counters.

### Task 2: Remaining raw SQL inventory

**Files:** Production Go files containing `Where`, `Select`, `Group`, `Having`, `Order`, `Joins`, `Raw`, or `Exec` with SQL expressions.

- [x] Enumerate SQL expression sites and separate GORM-managed simple columns from raw expressions.
- [x] Fix confirmed raw identifiers in the audited files using dialect quoting or `clause` expressions.
- [x] Add focused tests for query families where a failure would otherwise be silent.

### Task 3: Regression and safety check

**Files:** Tests and `docs/dm8-case-sensitive.md`.

- [x] Run focused real DM8 tests against the safe schema.
- [x] Run `go test ./...` without DM environment variables and `git diff --check`.
- [x] Document verified scope and any unverified paths precisely.

### Task 4: Independent review findings

- [x] Reproduce and fix unquoted identifiers in session confirmation, account binding, channel matching, and redemption status filters on real DM8.
- [x] Fix direct `First(..., "id = ?", ...)` conditions in redemption lookup and channel update; return channel reload errors.
- [x] Verify focused DM8 tests for these paths, including a channel update, without clearing the test schema.

### Task 5: Follow-up SQL expression review

- [x] Reinspect SQL expression arguments across production Go files, including expressions assembled from local variables and formatted strings.
- [x] Run the exact `/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20` request and tag mode against a real case-sensitive DM8 schema.
- [x] Extend non-destructive DM8 coverage for channel type counts, channel tags, capability joins, quota rankings, and log aggregates.
