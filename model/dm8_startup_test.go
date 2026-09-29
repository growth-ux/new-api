package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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
