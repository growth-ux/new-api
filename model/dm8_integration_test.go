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
	var caseSensitive int
	require.NoError(t, db.Raw("SELECT CASE_SENSITIVE()").Scan(&caseSensitive).Error)
	require.Equal(t, 1, caseSensitive, "DM8 integration tests require CASE_SENSITIVE=1")
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

// TestDM8StartupWithInitSQL verifies that the shipped SQL leaves a schema ready
// for repeated startup data initialization without any structural changes.
func TestDM8StartupWithInitSQL(t *testing.T) {
	db := openDM8TestDB(t)
	resetDM8Schema(t, db)
	recorder := &migrationSQLRecorder{}
	saveDM8Globals(t, db.Session(&gorm.Session{Logger: recorder}))

	for range 2 {
		require.NoError(t, migrateDB())
		require.NoError(t, MigrateAuditLogs())
	}
	require.Empty(t, recorder.schemaMutations())

	// The lock anchor seeded during migration must exist exactly once.
	var lockCount int64
	require.NoError(t, db.Model(&dm8TransactionLock{}).Where(dm8EmailLockPredicate, dm8EmailLockName).Count(&lockCount).Error)
	assert.EqualValues(t, 1, lockCount, "email lock anchor must be seeded once")

	unique, err := optionsKeyIsUnique(db)
	require.NoError(t, err)
	assert.True(t, unique, "options.key must be detected as a unique/primary key on DM8")
}

type dm8MetadataProbe struct {
	ID int64 `gorm:"primaryKey"`
}

func (dm8MetadataProbe) TableName() string { return "dm8_case_metadata_probe" }

// On a case-sensitive instance, migration must recognize the exact quoted
// lowercase table it created; otherwise the second migration tries to create
// that table again.
func TestDM8MigratorRecognizesQuotedTable(t *testing.T) {
	db := openDM8TestDB(t)
	defer func() { _ = db.Migrator().DropTable(&dm8MetadataProbe{}) }()

	require.NoError(t, db.AutoMigrate(&dm8MetadataProbe{}))
	require.True(t, db.Migrator().HasTable(&dm8MetadataProbe{}))
	require.NoError(t, db.AutoMigrate(&dm8MetadataProbe{}))
}

// These reads run during application startup and scheduled maintenance. A
// migration-only check cannot detect unquoted identifiers in their SQL.
func TestDM8StartupReadPaths(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)

	_, err := GetAllEnableAbilityWithChannels()
	require.NoError(t, err)
	_, err = ExpireDueSubscriptions(1)
	require.NoError(t, err)
	_, err = ResetDueSubscriptions(1)
	require.NoError(t, err)
	_, err = CountUserSessionsCreatedSince(0, 1)
	require.NoError(t, err)
	require.NoError(t, DeleteExpiredAuthFlows(time.Now()))
	require.NoError(t, ExpireStaleSystemTaskLocks(time.Now().Unix()))
	require.NoError(t, DeleteExpiredUserSessions(time.Now().Unix()))
	require.NoError(t, DeleteOldRevokedUserSessions(time.Now().Unix()))
	_, err = FindEarliestPendingSystemTasks([]string{SystemTaskTypeLogCleanup})
	require.NoError(t, err)
	_, err = GetLatestSystemTasks([]string{SystemTaskTypeLogCleanup})
	require.NoError(t, err)

	root := User{Username: "dm8-root-probe", Password: "unused-password", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(&root).Error })
	require.True(t, RootUserExists())

	midjourney := Midjourney{Progress: "50%"}
	require.NoError(t, db.Create(&midjourney).Error)
	t.Cleanup(func() { _ = db.Delete(&midjourney).Error })
	require.True(t, HasUnfinishedMidjourneyTasks())

	task := Task{Progress: "50%", Status: TaskStatusInProgress}
	require.NoError(t, db.Create(&task).Error)
	t.Cleanup(func() { _ = db.Delete(&task).Error })
	require.True(t, HasUnfinishedSyncTasks())
}

func TestDM8SystemTaskLifecycle(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)

	task, err := CreateSystemTask("dm8-case-task", map[string]string{"probe": "case1"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Delete(&SystemTaskLock{Type: task.Type}).Error
		_ = db.Delete(task).Error
	})

	claimed, acquired, err := ClaimSystemTask(task.ID, task.Type, "dm8-case-runner", time.Now().Unix()+60)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, task.TaskID, claimed.TaskID)
	require.NoError(t, UpdateSystemTaskState(task.TaskID, "dm8-case-runner", map[string]string{"step": "running"}))
	require.NoError(t, RenewSystemTaskLock(task.TaskID, "dm8-case-runner", time.Now().Unix()+120))
	require.NoError(t, FinishSystemTask(task.TaskID, "dm8-case-runner", SystemTaskStatusSucceeded, nil, ""))
	got, err := GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, SystemTaskStatusSucceeded, got.Status)
}

func TestDM8AbilityChannelSelection(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)

	tag := "dm8-case-tag"
	channel := Channel{Name: "dm8-case-channel", Key: "test-key", Models: "dm8-case-model", Group: "default", Tag: &tag, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	t.Cleanup(func() { _ = db.Delete(&channel).Error })
	ability := Ability{Group: "default", Model: "dm8-case-model", ChannelId: channel.Id, Enabled: true}
	require.NoError(t, db.Create(&ability).Error)
	t.Cleanup(func() { _ = db.Delete(&ability).Error })

	require.Contains(t, GetGroupEnabledModels("default"), ability.Model)
	require.Contains(t, GetEnabledModels(), ability.Model)
	require.NotEmpty(t, GetAllEnableAbilities())
	selected, err := GetChannel("default", ability.Model, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
	byID, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, channel.Name, byID.Name)
	searched, err := SearchChannels("dm8-case-channel", "default", "dm8-case-model", false)
	require.NoError(t, err)
	require.NotEmpty(t, searched)
	byTag, err := GetChannelsByTag(tag, false, true)
	require.NoError(t, err)
	require.NotEmpty(t, byTag)
	tags, err := GetPaginatedTags(0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, tags)
	searchedTags, err := SearchTags("dm8-case-channel", "default", "dm8-case-model", false)
	require.NoError(t, err)
	require.NotEmpty(t, searchedTags)
	byType, err := GetChannelsByType(0, 10, false, channel.Type)
	require.NoError(t, err)
	require.NotEmpty(t, byType)
	counts, err := CountChannelsGroupByType()
	require.NoError(t, err)
	require.Positive(t, counts[int64(channel.Type)])
}

func TestDM8UserRegistrationAndLogin(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)

	user := &User{Username: "dm8-case-user", Password: "case-password-123", Email: "dm8-case-user@example.test", Status: common.UserStatusEnabled}
	require.NoError(t, user.Insert(0))
	t.Cleanup(func() { _ = db.Unscoped().Delete(user).Error })

	exists, err := CheckUserExistOrDeleted(user.Username, user.Email)
	require.NoError(t, err)
	require.True(t, exists)
	count, err := CountUsersByEmail(user.Email)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	loaded, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, user.Username, loaded.Username)
	profile, err := GetSelfUserById(user.Id)
	require.NoError(t, err)
	require.True(t, profile.HasPassword)
	users, total, err := SearchUsers("dm8-case-user", "default", nil, nil, 0, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	require.NotEmpty(t, users)
	login := &User{Username: user.Username, Password: "case-password-123"}
	require.NoError(t, login.ValidateAndFill())
	require.Equal(t, user.Id, login.Id)
	cache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, user.Id, cache.Id)
	require.NoError(t, UpdateUserAccessToken(user.Id, "dm8-case-personal-token"))
	accessUser, err := ValidateAccessToken("dm8-case-personal-token")
	require.NoError(t, err)
	require.Equal(t, user.Id, accessUser.Id)
	quota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, user.Quota, quota)
	nextAuthVersion, err := BumpUserAuthVersion(user.Id)
	require.NoError(t, err)
	require.EqualValues(t, user.AuthVersion+1, nextAuthVersion)
}

func TestDM8UserSessionLifecycle(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)

	user := &User{Username: "dm8-session-user", Password: "unused-password", Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(user).Error })
	initialHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	nextHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	session := &UserSession{SID: "dm8-case-sid", UserID: user.Id, UserAuthVersion: 1, RefreshHash: initialHash, LoginMethod: "password", ExpiresAt: time.Now().Unix() + 3600}
	require.NoError(t, CreateUserSession(session))
	t.Cleanup(func() { _ = db.Delete(session).Error })
	count, err := CountActiveUserSessions(session.UserID, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	got, err := GetUserSessionCached(session.SID)
	require.NoError(t, err)
	require.Equal(t, session.SID, got.SID)
	listed, err := ListActiveUserSessions(session.UserID, session.SID, time.Now().Unix())
	require.NoError(t, err)
	require.NotEmpty(t, listed)
	rotated, err := RotateUserSessionRefresh(session.UserID, session.SID, session.RefreshHash, nextHash, time.Now().Unix(), time.Second)
	require.NoError(t, err)
	require.Equal(t, nextHash, rotated.RefreshHash)
	revoked, err := RevokeUserSession(session.UserID, session.SID, "dm8-test")
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestDM8AccessTokenLookup(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)
	token := &Token{UserId: 101, Key: "dm8-case-access-token", Name: "dm8-test", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())
	t.Cleanup(func() { _ = db.Unscoped().Delete(token).Error })
	validated, err := ValidateUserToken(token.Key)
	require.NoError(t, err)
	require.Equal(t, token.Id, validated.Id)
	byID, err := GetTokenByIds(token.Id, token.UserId)
	require.NoError(t, err)
	require.Equal(t, token.Key, byID.Key)
	require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 10))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 3))
	all, err := GetAllUserTokens(token.UserId, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, all)
	searched, total, err := SearchUserTokens(token.UserId, "dm8-test", "", 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, searched, 1)
}

func TestDM8QuotaReservation(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)
	user := &User{Username: "dm8-quota-user", Password: "unused-password", Quota: 100}
	require.NoError(t, db.Create(user).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(user).Error })
	token := &Token{UserId: user.Id, Key: "dm8-quota-token", Name: "quota", RemainQuota: 100, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(token).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(token).Error })

	ok, err := TryReserveUserQuota(user.Id, 30)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = TryReserveTokenQuota(token.Id, token.Key, 30, false)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = TryReserveUserQuota(user.Id, 100)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = TryReserveTokenQuota(token.Id, token.Key, 100, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, db.First(user, user.Id).Error)
	require.Equal(t, 70, user.Quota)
	require.NoError(t, db.First(token, token.Id).Error)
	require.Equal(t, 70, token.RemainQuota)
	require.Equal(t, 30, token.UsedQuota)
}

func TestDM8SubscriptionQueries(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)
	plan := &SubscriptionPlan{Title: "dm8-case-plan", Currency: "USD", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true}
	require.NoError(t, db.Create(plan).Error)
	t.Cleanup(func() { _ = db.Delete(plan).Error })
	sub := &UserSubscription{UserId: 101, PlanId: plan.Id, AmountTotal: 100, EndTime: time.Now().Unix() + 3600, Status: "active", AllowWalletOverflow: false}
	require.NoError(t, db.Create(sub).Error)
	t.Cleanup(func() { _ = db.Delete(sub).Error })

	loadedPlan, err := GetSubscriptionPlanById(plan.Id)
	require.NoError(t, err)
	require.Equal(t, plan.Id, loadedPlan.Id)
	count, err := CountUserSubscriptionsByPlan(sub.UserId, plan.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	active, err := GetAllActiveUserSubscriptions(sub.UserId)
	require.NoError(t, err)
	require.Len(t, active, 1)
	hasActive, err := HasActiveUserSubscription(sub.UserId)
	require.NoError(t, err)
	require.True(t, hasActive)
	allowOverflow, err := UserActiveSubscriptionsAllowWalletOverflow(sub.UserId)
	require.NoError(t, err)
	require.False(t, allowOverflow)
	consumed, err := PreConsumeUserSubscription("dm8-case-preconsume", sub.UserId, "dm8-case-model", 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 10, consumed.PreConsumed)
	t.Cleanup(func() {
		_ = db.Where(clause.Eq{Column: clause.Column{Name: "request_id"}, Value: "dm8-case-preconsume"}).Delete(&SubscriptionPreConsumeRecord{}).Error
	})
	require.NoError(t, RefundSubscriptionPreConsume("dm8-case-preconsume"))
}

func TestDM8AuthFlowLifecycle(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	resetDM8Schema(t, db)
	match := AuthFlowMatch{Purpose: "dm8-case-flow"}
	token, flow, err := CreateAuthFlow(AuthFlowCreate{Purpose: match.Purpose, ExpiresAt: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Delete(flow).Error })
	got, err := GetAuthFlow(token, match)
	require.NoError(t, err)
	require.Equal(t, flow.Id, got.Id)
	consumed, err := ConsumeAuthFlow(token, match)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

// Runs the exact projection used after password validation by POST /api/user/login.
// It inserts only one temporary user and does not migrate or clear the schema.
func TestDM8LoginVerificationProjection(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	probeID := time.Now().UnixNano()
	user := &User{Username: fmt.Sprintf("dm8probe%010d", probeID%1e10), AffCode: fmt.Sprintf("dm8-%d", probeID), Password: "hashed-password", Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(user).Error })

	state, err := GetUserVerificationState(user.Id)
	require.NoError(t, err)
	require.Equal(t, user.Id, state.UserID)
	require.Equal(t, user.AuthVersion, state.AuthVersion)
	require.True(t, state.HasPassword)
	require.False(t, state.HasTwoFA)
	require.False(t, state.HasPasskey)
	lockedUntil := time.Now().Add(time.Minute)
	twoFA := &TwoFA{UserId: user.Id, Secret: "dm8-probe-secret", IsEnabled: true, LockedUntil: &lockedUntil}
	require.NoError(t, db.Create(twoFA).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(twoFA).Error })
	state, err = GetUserVerificationState(user.Id)
	require.NoError(t, err)
	require.True(t, state.HasTwoFA)
	require.True(t, state.TwoFALocked)
	storedTwoFA, err := GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	require.Equal(t, twoFA.Id, storedTwoFA.Id)
}

// Runs the public performance-metric read, aggregate, upsert, and cleanup paths
// against a real CASE_SENSITIVE=1 schema. All writes are rolled back.
func TestDM8PerfMetricsCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)

	probeID := time.Now().UnixNano()
	modelName := fmt.Sprintf("dm8-perf-%d", probeID)
	group := fmt.Sprintf("dm8-perf-group-%d", probeID)
	bucket := time.Now().Unix()
	metric := &PerfMetric{
		ModelName: modelName, Group: group, BucketTs: bucket,
		RequestCount: 2, SuccessCount: 1, TotalLatencyMs: 100,
		OutputTokens: 20, GenerationMs: 10,
	}
	require.NoError(t, tx.Create(metric).Error)

	buckets, err := GetPerfMetricsSummaryBucketsAll(bucket, bucket, []string{group})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	require.Equal(t, modelName, buckets[0].ModelName)
	require.EqualValues(t, 2, buckets[0].RequestCount)

	summary, err := GetPerfMetricsSummaryAll(bucket, bucket, []string{group})
	require.NoError(t, err)
	require.Len(t, summary, 1)
	require.EqualValues(t, 1, summary[0].SuccessCount)

	metrics, err := GetPerfMetrics(modelName, group, bucket, bucket)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.EqualValues(t, 2, metrics[0].RequestCount)

	require.NoError(t, UpsertPerfMetric(&PerfMetric{ModelName: modelName, Group: group, BucketTs: bucket, RequestCount: 3, SuccessCount: 2, TotalLatencyMs: 60, OutputTokens: 8, GenerationMs: 4}))
	metrics, err = GetPerfMetrics(modelName, group, bucket, bucket)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.EqualValues(t, 5, metrics[0].RequestCount)
	require.EqualValues(t, 3, metrics[0].SuccessCount)

	require.NoError(t, DeletePerfMetricsBefore(bucket+1))
	metrics, err = GetPerfMetrics(modelName, group, bucket, bucket)
	require.NoError(t, err)
	require.Empty(t, metrics)
}

// Exercises the dashboard queries that aggregate quota_data with a quoted
// lowercase schema. All writes remain inside a rolled-back transaction.
func TestDM8QuotaDashboardCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)

	probeID := time.Now().UnixNano()
	username := fmt.Sprintf("dm8-quota-%d", probeID)
	modelName := fmt.Sprintf("dm8-model-%d", probeID)
	group := fmt.Sprintf("dm8-group-%d", probeID)
	bucket := time.Now().Unix() / 3600 * 3600
	row := &QuotaData{UserID: int(probeID%1e8) + 100000, Username: username, ModelName: modelName,
		CreatedAt: bucket, UseGroup: group, Count: 2, Quota: 10, TokenUsed: 4}
	require.NoError(t, tx.Create(row).Error)

	all, err := GetAllQuotaDates(bucket, bucket, "")
	require.NoError(t, err)
	require.NotEmpty(t, all)
	var found *QuotaData
	for _, item := range all {
		if item.ModelName == modelName {
			found = item
			break
		}
	}
	require.NotNil(t, found)
	require.Equal(t, 2, found.Count)
	require.Equal(t, 10, found.Quota)

	byName, err := GetQuotaDataByUsername(username, bucket, bucket)
	require.NoError(t, err)
	require.Len(t, byName, 1)
	byID, err := GetQuotaDataByUserId(row.UserID, bucket, bucket)
	require.NoError(t, err)
	require.Len(t, byID, 1)
	byUser, err := GetQuotaDataGroupByUser(bucket, bucket)
	require.NoError(t, err)
	require.NotEmpty(t, byUser)

	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		flows, err := GetFlowQuotaData(bucket, bucket, username, row.UserID, role)
		require.NoError(t, err)
		require.Len(t, flows, 1)
		require.Equal(t, modelName, flows[0].ModelName)
		require.Equal(t, 10, flows[0].Quota)
	}
	rankingTotals, err := GetRankingQuotaTotals(bucket, bucket)
	require.NoError(t, err)
	require.Contains(t, rankingTotals, RankingQuotaTotal{ModelName: modelName, TotalTokens: 4})
	rankingBuckets, err := GetRankingQuotaBuckets(bucket, bucket, 3600)
	require.NoError(t, err)
	require.Contains(t, rankingBuckets, RankingQuotaBucket{ModelName: modelName, Bucket: bucket, Tokens: 4})

	CacheQuotaDataLock.Lock()
	previousCache := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		CacheQuotaDataLock.Lock()
		CacheQuotaData = previousCache
		CacheQuotaDataLock.Unlock()
	})
	LogQuotaData(QuotaDataLogParams{UserID: row.UserID, Username: username, ModelName: modelName,
		CreatedAt: bucket, UseGroup: group, Quota: 5, TokenUsed: 3})
	SaveQuotaDataCache()
	var reloaded QuotaData
	require.NoError(t, tx.First(&reloaded, row.Id).Error)
	require.Equal(t, 3, reloaded.Count)
	require.Equal(t, 15, reloaded.Quota)
	require.Equal(t, 7, reloaded.TokenUsed)
}

func TestDM8AsyncTaskQueriesCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)

	probeID := time.Now().UnixNano()
	userID := int(probeID%1e8) + 200000
	publicID := fmt.Sprintf("dm8-task-%d", probeID)
	task := &Task{TaskID: publicID, UserId: userID, Status: TaskStatusInProgress, Progress: "50%", SubmitTime: time.Now().Unix()}
	require.NoError(t, tx.Create(task).Error)
	query := SyncTaskQueryParams{TaskID: publicID}
	require.Len(t, TaskGetAllUserTask(userID, 0, 10, query), 1)
	require.Len(t, TaskGetAllTasks(0, 10, query), 1)
	require.EqualValues(t, 1, TaskCountAllUserTask(userID, query))
	require.EqualValues(t, 1, TaskCountAllTasks(query))
	got, found, err := GetByOnlyTaskId(publicID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, task.ID, got.ID)

	mjID := fmt.Sprintf("dm8-mj-%d", probeID)
	mj := &Midjourney{UserId: userID, MjId: mjID, Progress: "50%", SubmitTime: time.Now().Unix()}
	require.NoError(t, tx.Create(mj).Error)
	mjQuery := TaskQueryParams{MjID: mjID}
	require.Len(t, GetAllUserTask(userID, 0, 10, mjQuery), 1)
	require.Len(t, GetAllTasks(0, 10, mjQuery), 1)
	require.EqualValues(t, 1, CountAllUserTask(userID, mjQuery))
	require.EqualValues(t, 1, CountAllTasks(mjQuery))
	require.NotNil(t, GetByOnlyMJId(mjID))
}

func TestDM8CatalogQueriesCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	_, err := GetAllEnableAbilityWithChannels()
	require.NoError(t, err)
	_, err = GetModelConnections()
	require.NoError(t, err)
	_, err = GetPreferredModelOwnerChannelTypes([]string{"dm8-missing-probe"}, []string{"default"})
	require.NoError(t, err)
	_, _, err = SearchModels("dm8-missing-probe", "", "", "", 0, 10)
	require.NoError(t, err)
	_, err = GetVendorModelCounts()
	require.NoError(t, err)
}

func TestDM8AuditQueriesCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	_, _, err := GetAuditLogs(AuditLogFilter{Username: "dm8-missing-probe", Category: AuditCategoryLogin}, 0, 10, common.RoleCommonUser)
	require.NoError(t, err)
	_, err = GetLogByTokenId(0)
	require.NoError(t, err)
}

func TestDM8LogAggregateCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)
	probeID := time.Now().UnixNano()
	username := fmt.Sprintf("dm8-log-%d", probeID)
	modelName := fmt.Sprintf("dm8-log-model-%d", probeID)
	log := &Log{Type: LogTypeConsume, Quota: 100, PromptTokens: 10, CompletionTokens: 5,
		CreatedAt: time.Now().Unix(), UserId: int(probeID%1e8) + 500000, Username: username, ModelName: modelName}
	require.NoError(t, tx.Create(log).Error)
	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, modelName, username, "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 100, stat.Quota)
	require.Equal(t, 15, SumUsedToken(LogTypeConsume, 0, 0, modelName, username, ""))
	logs, total, err := GetAllLogs(LogTypeConsume, 0, 0, modelName, username, "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
}

func TestDM8TopUpQueriesCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	probeID := time.Now().UnixNano()
	tradeNo := fmt.Sprintf("dm8-topup-%d", probeID)
	topUp := &TopUp{UserId: int(probeID%1e8) + 300000, TradeNo: tradeNo, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(topUp).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(topUp).Error })
	require.NotNil(t, GetTopUpByTradeNo(tradeNo))
	page := &common.PageInfo{Page: 1, PageSize: 10}
	rows, _, err := SearchAllTopUps(tradeNo, page)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestDM8CheckinStatsCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)
	userID := int(time.Now().UnixNano()%1e8) + 400000
	date := time.Now().Format("2006-01-02")
	require.NoError(t, tx.Create(&Checkin{UserId: userID, CheckinDate: date, QuotaAwarded: 8, CreatedAt: time.Now().Unix()}).Error)
	stats, err := GetUserCheckinStats(userID, date[:7])
	require.NoError(t, err)
	require.EqualValues(t, 8, stats["total_quota"])
}

func TestDM8AdminReadQueriesCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	_, err := CountChannelsGroupByType()
	require.NoError(t, err)
	_, err = GetRedemptionById(2147483647)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, _, err = GetAllRedemptions(0, 10)
	require.NoError(t, err)
	_, _, err = SearchRedemptions("dm8-missing-probe", "", 0, 10)
	require.NoError(t, err)
	_, _, err = SearchRedemptions("dm8-missing-probe", "expired", 0, 10)
	require.NoError(t, err)
	_, _, err = SearchRedemptions("dm8-missing-probe", fmt.Sprint(common.RedemptionCodeStatusEnabled), 0, 10)
	require.NoError(t, err)
	_, _, err = SearchVendors("dm8-missing-probe", 0, 10)
	require.NoError(t, err)
	_, err = GetAllPrefillGroups("dm8-missing-probe")
	require.NoError(t, err)
	_, err = ListSystemInstances()
	require.NoError(t, err)
	_, _, err = GetTaskPluginUsage("dm8-missing-probe")
	require.NoError(t, err)
	_, err = ListTaskPluginVersions("dm8-missing-probe")
	require.NoError(t, err)
	_, err = GetAllCustomOAuthProviders()
	require.NoError(t, err)
	_, err = GetEnabledCustomOAuthProviders()
	require.NoError(t, err)
}

func TestDM8ChannelSatisfyCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)
	ability := Ability{Group: fmt.Sprintf("dm8-check-%d", time.Now().UnixNano()), Model: "dm8-check-model", ChannelId: 987654, Enabled: true}
	require.NoError(t, tx.Create(&ability).Error)
	require.True(t, isChannelEnabledForGroupModelDB(ability.Group, ability.Model, ability.ChannelId))
}

func TestDM8ChannelUpdateCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)
	tag := fmt.Sprintf("dm8-tag-%d", time.Now().UnixNano())
	channel := &Channel{Name: fmt.Sprintf("dm8-update-%d", time.Now().UnixNano()), Key: "dm8-test-key", Models: "dm8-update-model", Group: "default", Tag: &tag, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	t.Cleanup(func() {
		_ = db.Where(clause.Eq{Column: clause.Column{Name: "channel_id"}, Value: channel.Id}).Delete(&Ability{}).Error
		_ = db.Unscoped().Delete(channel).Error
	})
	updatedName := channel.Name + "-edited"
	channel.Name = updatedName
	require.NoError(t, channel.Update())
	require.Equal(t, updatedName, channel.Name)
	require.Contains(t, GetGroupEnabledModels("default"), channel.Models)
	abilityRows, err := GetAllEnableAbilityWithChannels()
	require.NoError(t, err)
	require.NotEmpty(t, abilityRows)
	tags, err := GetPaginatedTags(0, 100)
	require.NoError(t, err)
	require.Contains(t, tags, &tag)
	channels, err := GetChannelsByTag(tag, false, true)
	require.NoError(t, err)
	require.NotEmpty(t, channels)
	searchedTags, err := SearchTags(updatedName, "default", channel.Models, false)
	require.NoError(t, err)
	require.Contains(t, searchedTags, &tag)
}

func TestDM8SessionSnapshotAndBindingCaseSensitive(t *testing.T) {
	db := openDM8TestDB(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	saveDM8Globals(t, tx)
	probeID := time.Now().UnixNano()
	user := &User{Username: fmt.Sprintf("dm8bind%010d", probeID%1e10), AffCode: fmt.Sprintf("dm8-bind-%d", probeID), Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, tx.Create(user).Error)
	session := &UserSession{SID: fmt.Sprintf("dm8-bind-%d", probeID), UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, RefreshHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", LoginMethod: "password", ExpiresAt: time.Now().Unix() + 3600}
	require.NoError(t, tx.Create(session).Error)
	require.NoError(t, confirmUserSessionActiveSnapshot(session))
	identity := AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
	require.NoError(t, UpdateUserBindColumnForSessionWithTx(tx, identity, "github_id", fmt.Sprintf("dm8-bind-%d", probeID)))
}

// TestDM8PrefillSoftDeleteUniqueness verifies the CASE-expression index enforces
// uniqueness only among non-deleted rows, matching the PostgreSQL partial index.
func TestDM8PrefillSoftDeleteUniqueness(t *testing.T) {
	db := openDM8TestDB(t)
	saveDM8Globals(t, db)

	resetDM8Schema(t, db)

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
		CreatedAt: now, UserId: 101, Username: "dm8-stat-user", ModelName: "model-a",
	}).Error)

	stat, err := SumUsedQuota(LogTypeConsume, 0, 0, "", "", "", 0, "")
	require.NoError(t, err, "log stat aggregate scan must succeed on DM8")
	assert.Equal(t, 100, stat.Quota)
	all, total, err := GetAllLogs(LogTypeConsume, 0, 0, "model-a", "dm8-stat-user", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	require.NotEmpty(t, all)
	userLogs, userTotal, err := GetUserLogs(101, LogTypeConsume, 0, 0, "model-a", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, userTotal, int64(1))
	require.NotEmpty(t, userLogs)
	require.Equal(t, 15, SumUsedToken(LogTypeConsume, 0, 0, "model-a", "dm8-stat-user", ""))
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
	require.NoError(t, db.Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: "dm8-role"}).Find(&roles).Error)
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
