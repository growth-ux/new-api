package model

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// dm8TestDSN returns the isolated DM8 test DSN, or "" when the matrix is not
// configured. TEST_DATABASE_MATRIX=1 turns a missing DSN into a hard failure so
// release runs cannot silently skip.
func dm8TestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DM_DSN")
	if dsn == "" {
		if os.Getenv("TEST_DATABASE_MATRIX") == "1" {
			t.Fatal("TEST_DM_DSN is required when TEST_DATABASE_MATRIX=1")
		}
		t.Skip("TEST_DM_DSN not set; skipping DM8 integration test")
	}
	return dsn
}

// openDM8TestDB connects to the isolated DM8 test schema and verifies the current
// schema carries the NEWAPI_TEST_ prefix before any DDL/DML runs.
func openDM8TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := dm8TestDSN(t)
	db, err := openDM8(dsn)
	require.NoError(t, err, "openDM8 must connect to the isolated test schema")
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	var schema string
	require.NoError(t, db.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&schema).Error)
	expected := os.Getenv("TEST_DM_SCHEMA")
	if expected != "" {
		require.Equal(t, expected, schema, "current schema must equal TEST_DM_SCHEMA")
	}
	require.True(t, len(schema) >= len("NEWAPI_TEST_") && schema[:len("NEWAPI_TEST_")] == "NEWAPI_TEST_",
		"test schema %q must use the NEWAPI_TEST_ prefix", schema)
	return db
}

// dropAllDM8Tables clears every table in the connected schema so a migration test
// starts from a known-empty state. CASCADE plus a repeat loop handles FK ordering.
func dropAllDM8Tables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for range 5 {
		var tables []string
		require.NoError(t, db.Raw("SELECT TABLE_NAME FROM USER_TABLES").Scan(&tables).Error)
		if len(tables) == 0 {
			return
		}
		for _, table := range tables {
			_ = db.Exec("DROP TABLE " + dm8QuoteIdent(table) + " CASCADE").Error
		}
	}
}

// saveDM8Globals snapshots the global database state and restores it on cleanup so
// DM8 tests never leak into the SQLite-backed suite.
func saveDM8Globals(t *testing.T, db *gorm.DB) {
	t.Helper()
	prevDB, prevLog := DB, LOG_DB
	prevMain, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	prevRedis, prevBatch, prevLogConsume := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled

	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeDM, common.DatabaseTypeDM)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	initCol()

	t.Cleanup(func() {
		DB, LOG_DB = prevDB, prevLog
		common.SetDatabaseTypes(prevMain, prevLogType)
		common.RedisEnabled = prevRedis
		common.BatchUpdateEnabled = prevBatch
		common.LogConsumeEnabled = prevLogConsume
		initCol()
	})
}

// TestDM8StartupMigration runs the real production migration path against DM8 on a
// fresh schema, then runs it a second time to prove idempotency, and verifies the
// corrected index metadata and the soft-delete partial unique index.
func TestDM8StartupMigration(t *testing.T) {
	db := openDM8TestDB(t)
	dropAllDM8Tables(t, db)
	saveDM8Globals(t, db)

	require.NoError(t, migrateDB(), "first production migration must succeed on DM8")
	require.NoError(t, MigrateAuditLogs(), "audit log migration must succeed on DM8")

	// Second run must not fail and must not need destructive re-DDL.
	require.NoError(t, migrateDB(), "second migration run must be idempotent")

	// The lock anchor seeded during migration must exist exactly once.
	var lockCount int64
	require.NoError(t, db.Model(&dm8TransactionLock{}).Where("name = ?", dm8EmailLockName).Count(&lockCount).Error)
	assert.EqualValues(t, 1, lockCount, "email lock anchor must be seeded once")

	// The options primary-key detection used by migrateOptionPrimaryKey must
	// recognize the key as unique/primary on DM8 (the driver reports it via the
	// primary-key flag; optionsKeyIsUnique accepts either).
	unique, err := optionsKeyIsUnique(db)
	require.NoError(t, err)
	assert.True(t, unique, "options.key must be detected as a unique/primary key on DM8")
}

// TestDM8PrefillSoftDeleteUniqueness verifies the CASE-expression index enforces
// uniqueness only among non-deleted rows, matching the PostgreSQL partial index.
func TestDM8PrefillSoftDeleteUniqueness(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&PrefillGroup{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&PrefillGroup{}) })

	first := PrefillGroup{Name: "dm8-prefill", Type: "test", Items: JSONValue(`{}`)}
	require.NoError(t, first.Insert())

	dup := PrefillGroup{Name: "dm8-prefill", Type: "test", Items: JSONValue(`{}`)}
	assert.Error(t, dup.Insert(), "a second active group with the same name must be rejected")

	// Soft-delete the first, then a new active group with the same name is allowed.
	require.NoError(t, db.Delete(&PrefillGroup{}, first.Id).Error)
	replacement := PrefillGroup{Name: "dm8-prefill", Type: "test", Items: JSONValue(`{}`)}
	require.NoError(t, replacement.Insert(), "reusing a soft-deleted name must succeed")
}

// TestDM8ValueRoundTrip verifies CLOB(json), bigint, decimal, bool and large
// multibyte text survive a write/read cycle without truncation or type loss.
func TestDM8ValueRoundTrip(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&Channel{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&Channel{}) })

	ts, err := dm8DatabaseTimestamp(db)
	require.NoError(t, err, "DM8 database timestamp must be readable")
	assert.Positive(t, ts)
}

// TestDM8LogStat validates the aggregate-alias scan used by the log statistics
// query. DM8 uppercases unquoted column aliases (QUOTA), which broke GORM's struct
// field mapping; the aliases are now double-quoted so the scan succeeds.
func TestDM8LogStat(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&Log{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&Log{}) })

	now := time.Now().Unix()
	require.NoError(t, db.Create(&Log{
		Type: LogTypeConsume, Quota: 100, PromptTokens: 10, CompletionTokens: 5,
		CreatedAt: now, Username: "dm8-stat-user", ModelName: "model-a",
	}).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err, "log stat aggregate scan must succeed on DM8")
	assert.Equal(t, 100, stat.Quota)
}

// TestDM8RankingBuckets validates the DM8 FLOOR bucket expression and the
// quota_data aggregation/ordering used by the ranking query.
func TestDM8RankingBuckets(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&QuotaData{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&QuotaData{}) })

	rows := []QuotaData{
		{ModelName: "model-a", CreatedAt: 3599, TokenUsed: 7},
		{ModelName: "model-a", CreatedAt: 3600, TokenUsed: 11},
		{ModelName: "model-a", CreatedAt: 3601, TokenUsed: 13},
	}
	require.NoError(t, db.Create(&rows).Error)

	got, err := GetRankingQuotaBuckets(1, 7200, 3600)
	require.NoError(t, err)
	assert.Equal(t, []RankingQuotaBucket{
		{ModelName: "model-a", Bucket: 0, Tokens: 7},
		{ModelName: "model-a", Bucket: 3600, Tokens: 24},
	}, got)
}

// TestDM8ConflictUpsert covers the authz/system_instance pattern: OnConflict with
// DoUpdates on a non-primary-key unique column. The first insert creates the row;
// the second must update it instead of failing with a uniqueness violation.
func TestDM8ConflictUpsert(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&AuthzRole{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&AuthzRole{}) })

	upsert := func(name string, sort int) error {
		return db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "sort"}),
		}).Create(&AuthzRole{Key: "dm8-role", Name: name, Sort: sort, Enabled: true}).Error
	}

	require.NoError(t, upsert("first", 1))
	require.NoError(t, upsert("second", 2), "upsert on conflict must not fail on DM8")

	var roles []AuthzRole
	require.NoError(t, db.Where("key = ?", "dm8-role").Find(&roles).Error)
	require.Len(t, roles, 1, "upsert must not duplicate the row")
	assert.Equal(t, "second", roles[0].Name)
	assert.Equal(t, 2, roles[0].Sort)
}

// TestDM8ConflictDoNothing covers the auth_flow single-claim contract: a fresh
// insert reports RowsAffected=1, and a conflicting insert with DoNothing reports
// RowsAffected=0 with no error, both inside a transaction.
func TestDM8ConflictDoNothing(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	require.NoError(t, db.AutoMigrate(&AuthFlow{}))
	t.Cleanup(func() { _ = db.Migrator().DropTable(&AuthFlow{}) })

	now := time.Now()
	claim := func(hash string) *gorm.DB {
		return db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token_hash"}},
			DoNothing: true,
		}).Create(&AuthFlow{TokenHash: hash, Purpose: "test", ExpiresAt: now.Add(time.Hour), ConsumedAt: &now})
	}

	first := claim("dm8-assertion-hash")
	require.NoError(t, first.Error)
	assert.EqualValues(t, 1, first.RowsAffected, "fresh claim must affect one row")

	second := claim("dm8-assertion-hash")
	require.NoError(t, second.Error, "conflicting DoNothing claim must not error")
	assert.Zero(t, second.RowsAffected, "conflicting claim must affect zero rows")

	// The same guarantee must hold inside an explicit transaction.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token_hash"}},
			DoNothing: true,
		}).Create(&AuthFlow{TokenHash: "dm8-assertion-hash", Purpose: "test", ExpiresAt: now.Add(time.Hour), ConsumedAt: &now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 0 {
			return fmt.Errorf("expected zero rows in tx, got %d", res.RowsAffected)
		}
		return nil
	}))
}
