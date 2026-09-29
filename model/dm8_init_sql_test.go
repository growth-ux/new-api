package model

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	dm8LogSectionBegin = "-- BEGIN LOG SCHEMA"
	dm8LogSectionEnd   = "-- END LOG SCHEMA"
)

func resetDM8Schema(t *testing.T, db *gorm.DB) {
	t.Helper()
	dropAllDM8Tables(t, db)
	applyDM8InitSQL(t, db, "all")
}

// applyDM8InitSQL runs the shipped SQL in a protected test schema only.
func applyDM8InitSQL(t *testing.T, db *gorm.DB, section string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	sqlFile := filepath.Join(filepath.Dir(file), "..", "database", "dm8", "init.sql")
	contents, err := os.ReadFile(sqlFile)
	require.NoError(t, err)
	var selected []string
	inLog := false
	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case dm8LogSectionBegin:
			inLog = true
			continue
		case dm8LogSectionEnd:
			inLog = false
			continue
		}
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}
		if section == "all" || section == "log" && inLog {
			selected = append(selected, line)
		}
	}
	require.Contains(t, []string{"all", "log"}, section)
	for _, statement := range strings.Split(strings.Join(selected, "\n"), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		require.NoError(t, db.Exec(statement).Error, "init.sql statement: %s", statement)
	}
}

func TestDM8InitSQLComplete(t *testing.T) {
	db := openDM8TestDB(t)
	dropAllDM8Tables(t, db)
	applyDM8InitSQL(t, db, "all")
	for _, table := range []any{
		&Channel{}, &Token{}, &User{}, &UserSession{}, &AuthFlow{},
		&ExternalIdentityClaim{}, &PasskeyCredential{}, &Option{}, &LoginEncryptionKey{},
		&Redemption{}, &Ability{}, &Log{}, &Midjourney{}, &TopUp{}, &QuotaData{},
		&Task{}, &TaskPlugin{}, &Model{}, &Vendor{}, &PrefillGroup{}, &Setup{},
		&TwoFA{}, &TwoFABackupCode{}, &Checkin{}, &SubscriptionOrder{},
		&UserSubscription{}, &SubscriptionPreConsumeRecord{}, &CustomOAuthProvider{},
		&UserOAuthBinding{}, &PerfMetric{}, &SystemInstance{}, &SystemTask{},
		&SystemTaskLock{}, &CasbinRule{}, &AuthzRole{}, &SubscriptionPlan{},
		&AuditLog{}, &dm8TransactionLock{},
	} {
		require.True(t, db.Migrator().HasTable(table), "missing table for %T", table)
		statement := &gorm.Statement{DB: db}
		require.NoError(t, statement.Parse(table))
		for _, field := range statement.Schema.Fields {
			if field.DBName != "" && !field.IgnoreMigration {
				require.True(t, db.Migrator().HasColumn(table, field.DBName), "missing %s.%s", statement.Schema.Table, field.DBName)
			}
		}
		for name := range statement.Schema.ParseIndexes() {
			require.True(t, db.Migrator().HasIndex(table, name), "missing index %s on %s", name, statement.Schema.Table)
		}
	}
	var count int64
	require.NoError(t, db.Model(&dm8TransactionLock{}).Where(dm8EmailLockPredicate, dm8EmailLockName).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var quotaType string
	for _, column := range []string{"quota", "used_quota", "aff_quota", "aff_history"} {
		columns, err := db.Migrator().ColumnTypes(&User{})
		require.NoError(t, err)
		quotaType = ""
		for _, actual := range columns {
			if actual.Name() == column {
				quotaType = actual.DatabaseTypeName()
				break
			}
		}
		require.Equal(t, "BIGINT", quotaType, "users.%s", column)
	}
	user := User{Username: "dm8-sql-quota", Password: "unused", Quota: 1 << 40, UsedQuota: 1 << 40, AffQuota: 1 << 40, AffHistoryQuota: 1 << 40}
	require.NoError(t, db.Create(&user).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(&user).Error })
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 1<<40, stored.Quota)
	require.Equal(t, 1<<40, stored.UsedQuota)
	require.Equal(t, 1<<40, stored.AffQuota)
	require.Equal(t, 1<<40, stored.AffHistoryQuota)
}

func TestDM8InitSQLLogSection(t *testing.T) {
	db := openDM8TestDB(t)
	dropAllDM8Tables(t, db)
	t.Cleanup(func() {
		dropAllDM8Tables(t, db)
		applyDM8InitSQL(t, db, "all")
	})
	applyDM8InitSQL(t, db, "log")
	require.True(t, db.Migrator().HasTable(&Log{}))
	require.True(t, db.Migrator().HasTable(&AuditLog{}))
	require.False(t, db.Migrator().HasTable(&User{}))
	require.False(t, db.Migrator().HasTable(&dm8TransactionLock{}))
}
