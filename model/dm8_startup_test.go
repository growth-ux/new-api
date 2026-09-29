package model

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func requiredDM8TestEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		if os.Getenv("TEST_DATABASE_MATRIX") == "1" {
			t.Fatal(name + " is required when TEST_DATABASE_MATRIX=1")
		}
		t.Skip(name + " is not configured")
	}
	return value
}

func grantDM8RuntimeTestTables(t *testing.T, owner *gorm.DB, runtimeUser string) {
	t.Helper()
	require.True(t, strings.HasPrefix(runtimeUser, "NEWAPI_TEST_"))
	var schema string
	require.NoError(t, owner.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&schema).Error)
	require.True(t, strings.HasPrefix(schema, "NEWAPI_TEST_"))
	var tables []string
	require.NoError(t, owner.Raw("SELECT TABLE_NAME FROM USER_TABLES").Scan(&tables).Error)
	require.NotEmpty(t, tables)
	for _, table := range tables {
		statement := fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON %s.%s TO %s", dm8QuoteIdent(schema), dm8QuoteIdent(table), dm8QuoteIdent(runtimeUser))
		require.NoError(t, owner.Exec(statement).Error)
	}
}

func TestDM8StartupWithRuntimeAccount(t *testing.T) {
	runtimeDSN := requiredDM8TestEnv(t, "TEST_DM_RUNTIME_DSN")
	runtimeLogDSN := requiredDM8TestEnv(t, "TEST_DM_RUNTIME_LOG_DSN")
	logOwnerDSN := requiredDM8TestEnv(t, "TEST_LOG_DM_DSN")
	parsed, err := url.Parse(runtimeDSN)
	require.NoError(t, err)
	runtimeUser := parsed.User.Username()
	owner := openDM8TestDB(t)
	resetDM8Schema(t, owner)
	grantDM8RuntimeTestTables(t, owner, runtimeUser)
	logOwner, err := openDM8(logOwnerDSN)
	require.NoError(t, err)
	logOwnerPool, err := logOwner.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = logOwnerPool.Close() })
	var logSchema string
	require.NoError(t, logOwner.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&logSchema).Error)
	require.Equal(t, requiredDM8TestEnv(t, "TEST_LOG_DM_SCHEMA"), logSchema)
	dropAllDM8Tables(t, logOwner)
	applyDM8InitSQL(t, logOwner, "log")
	grantDM8RuntimeTestTables(t, logOwner, runtimeUser)

	probe, err := openDM8(runtimeDSN)
	require.NoError(t, err)
	probePool, err := probe.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = probePool.Close() })
	var count int64
	var systemPrivileges []string
	require.NoError(t, probe.Raw("SELECT PRIVILEGE FROM SESSION_PRIVS").Scan(&systemPrivileges).Error)
	require.NotContains(t, systemPrivileges, "SELECT ANY DICTIONARY")
	probeTable := `"newapi_runtime_privilege_probe"`
	createErr := probe.Exec("CREATE TABLE " + probeTable + ` ("id" BIGINT)`).Error
	if createErr == nil {
		_ = probe.Exec("DROP TABLE " + probeTable).Error
		t.Fatal("runtime identity unexpectedly has CREATE TABLE privilege")
	}
	user := User{Username: "dm8-runtime-crud", Password: "unused", AffCode: "dm8-runtime-crud", Email: "dm8-runtime-crud@example.test"}
	require.NoError(t, probe.Create(&user).Error)
	t.Cleanup(func() { _ = probe.Unscoped().Delete(&user).Error })
	var stored User
	require.NoError(t, probe.First(&stored, user.Id).Error)
	require.NoError(t, probe.Model(&stored).Update("remark", "runtime-write-ok").Error)

	for _, tc := range []struct {
		name, mainDSN, logDSN string
		master                bool
	}{
		{"dm_shared_master", runtimeDSN, "", true},
		{"dm_shared_slave", runtimeDSN, "", false},
		{"dm_separate_dm_log", runtimeDSN, runtimeLogDSN, true},
		{"dm_sqlite_log", runtimeDSN, "local", true},
		{"sqlite_dm_log", "local", runtimeLogDSN, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldDB, oldLog := DB, LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldMaster, oldSQLitePath := common.IsMasterNode, common.SQLitePath
			t.Cleanup(func() {
				if LOG_DB != nil && LOG_DB != DB && LOG_DB != oldLog {
					if pool, err := LOG_DB.DB(); err == nil {
						_ = pool.Close()
					}
				}
				if DB != nil && DB != oldDB {
					if pool, err := DB.DB(); err == nil {
						_ = pool.Close()
					}
				}
				DB, LOG_DB = oldDB, oldLog
				common.SetDatabaseTypes(oldMainType, oldLogType)
				common.IsMasterNode, common.SQLitePath = oldMaster, oldSQLitePath
				initCol()
			})
			t.Setenv("SQL_DSN", tc.mainDSN)
			t.Setenv("LOG_SQL_DSN", tc.logDSN)
			common.SQLitePath = filepath.Join(t.TempDir(), "mixed.db")
			common.IsMasterNode = tc.master
			require.NoError(t, InitDB())
			require.NoError(t, InitLogDB())
			if tc.logDSN == runtimeLogDSN {
				require.NoError(t, LOG_DB.Raw(`SELECT COUNT(*) FROM "audit_logs"`).Scan(&count).Error)
			}
		})
	}
}

func TestDM8EmptySchemaDoesNotRepair(t *testing.T) {
	emptyDSN := requiredDM8TestEnv(t, "TEST_DM_EMPTY_DSN")
	db, err := openDM8(emptyDSN)
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	var schema string
	require.NoError(t, db.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&schema).Error)
	require.True(t, strings.HasPrefix(schema, "NEWAPI_TEST_"))
	var count int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM USER_TABLES").Scan(&count).Error)
	require.Zero(t, count)
	previousDB, previousLog := DB, LOG_DB
	previousMain, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeDM, common.DatabaseTypeDM)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLog
		common.SetDatabaseTypes(previousMain, previousLogType)
	})
	require.Error(t, migrateDB(), "empty DM schema must not be repaired at startup")
	require.NoError(t, migrateLOGDB())
	require.NoError(t, MigrateAuditLogs())
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM USER_TABLES").Scan(&count).Error)
	require.Zero(t, count)
}

func TestDM8EmailUniquenessWithInitSQL(t *testing.T) {
	db := openDM8TestDB(t)
	resetDM8Schema(t, db)
	saveDM8Globals(t, db)
	name := fmt.Sprintf("dm8-email-%d", time.Now().UnixNano())
	email := name + "@example.test"
	users := []*User{
		{Username: name + "a", Password: "password-123", Email: strings.ToUpper(email)},
		{Username: name + "b", Password: "password-123", Email: email},
	}
	start := make(chan struct{})
	results := make([]error, 2)
	var group sync.WaitGroup
	for i := range users {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index] = users[index].Insert(0)
		}(i)
	}
	close(start)
	group.Wait()
	success, occupied := 0, 0
	for _, err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrEmailAlreadyTaken) {
			occupied++
		} else {
			t.Fatalf("unexpected concurrent email result: %v", err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, occupied)
	var count int64
	require.NoError(t, db.Model(&User{}).Where(`"email" = ?`, email).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestDM8StartupSkipsSchemaWork(t *testing.T) {
	db := openDM8TestDB(t)
	resetDM8Schema(t, db)
	recorder := &migrationSQLRecorder{}
	observed := db.Session(&gorm.Session{Logger: recorder})
	saveDM8Globals(t, observed)

	require.NoError(t, checkDM8Compatibility(observed))
	beforeQuota := len(recorder.statements)
	require.NoError(t, ensureUserQuotaColumns(observed, common.DatabaseTypeDM))
	require.Len(t, recorder.statements, beforeQuota, "DM quota check must not query table metadata")
	require.NoError(t, migrateDB())
	require.NoError(t, migrateLOGDB())
	require.NoError(t, MigrateAuditLogs())
	require.Empty(t, recorder.schemaMutations(), "DM startup must not change schema")
	for _, statement := range recorder.statements {
		upper := strings.ToUpper(statement)
		for _, catalog := range []string{"SYS.SYSOBJECTS", "USER_TABLES", "USER_TAB_COLUMNS", "USER_IND_COLUMNS"} {
			require.NotContains(t, upper, catalog, "DM startup queried migration metadata")
		}
	}
}

func TestDM8StartupPreservesDataInitialization(t *testing.T) {
	db := openDM8TestDB(t)
	resetDM8Schema(t, db)
	saveDM8Globals(t, db)
	first := User{Username: "dm8-legacy-telegram-1", Password: "unused", TelegramId: "dm8-legacy-telegram-1", AffCode: "dm8-legacy-aff-1"}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Model(&first).Update("auth_version", 0).Error)

	require.NoError(t, migrateDB())
	require.NoError(t, migrateDB())
	var stored User
	require.NoError(t, db.First(&stored, first.Id).Error)
	require.EqualValues(t, 1, stored.AuthVersion)
	var count int64
	require.NoError(t, db.Model(&ExternalIdentityClaim{}).Where(`"provider" = ? AND "subject" = ? AND "user_id" = ?`, ExternalIdentityProviderTelegram, first.TelegramId, first.Id).Count(&count).Error)
	require.EqualValues(t, 1, count)

	second := User{Username: "dm8-legacy-telegram-2", Password: "unused", TelegramId: "dm8-legacy-telegram-2", AffCode: "dm8-legacy-aff-2"}
	require.NoError(t, db.Create(&second).Error)
	require.NoError(t, db.Model(&second).Update("auth_version", 0).Error)
	wasMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = wasMaster })
	t.Setenv("SQL_DSN", dm8TestDSN(t))
	require.NoError(t, InitDB())
	pool, err := DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	stored = User{}
	require.NoError(t, db.First(&stored, second.Id).Error)
	require.EqualValues(t, 0, stored.AuthVersion, "slave startup must not run data backfill")
	require.NoError(t, db.Model(&ExternalIdentityClaim{}).Where(`"provider" = ? AND "subject" = ?`, ExternalIdentityProviderTelegram, second.TelegramId).Count(&count).Error)
	require.Zero(t, count)
}

func TestStartupMigrationDialectRouting(t *testing.T) {
	for _, tc := range []struct {
		name, main, log string
	}{
		{"dm_dm", "dm", "dm"},
		{"dm_sqlite", "dm", "sqlite"},
		{"sqlite_dm", "sqlite", "dm"},
		{"sqlite_sqlite", "sqlite", "sqlite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dmDB *gorm.DB
			if tc.main == "dm" || tc.log == "dm" {
				dmDB = openDM8TestDB(t)
				resetDM8Schema(t, dmDB)
			}
			mainDB, mainRecorder := startupRoutingDB(t, tc.main, dmDB)
			logDB, logRecorder := startupRoutingDB(t, tc.log, dmDB)
			previousDB, previousLog := DB, LOG_DB
			previousMain, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = mainDB, logDB
			mainType, logType := common.DatabaseTypeSQLite, common.DatabaseTypeSQLite
			if tc.main == "dm" {
				mainType = common.DatabaseTypeDM
			}
			if tc.log == "dm" {
				logType = common.DatabaseTypeDM
			}
			common.SetDatabaseTypes(mainType, logType)
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLog
				common.SetDatabaseTypes(previousMain, previousLogType)
			})
			require.NoError(t, migrateDB())
			require.NoError(t, migrateLOGDB())
			if tc.main == "dm" {
				require.Empty(t, mainRecorder.schemaMutations())
			} else {
				require.NotEmpty(t, mainRecorder.schemaMutations())
			}
			if tc.log == "dm" {
				require.Empty(t, logRecorder.schemaMutations())
			} else {
				require.NotEmpty(t, logRecorder.schemaMutations())
			}
		})
	}
}

func startupRoutingDB(t *testing.T, dialect string, dmDB *gorm.DB) (*gorm.DB, *migrationSQLRecorder) {
	t.Helper()
	recorder := &migrationSQLRecorder{}
	if dialect == "dm" {
		return dmDB.Session(&gorm.Session{Logger: recorder}), recorder
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: recorder})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	return db, recorder
}
