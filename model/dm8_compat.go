package model

import (
	"fmt"

	"gorm.io/gorm"
)

// dm8UnixTimestampSQL returns UTC Unix seconds from DM8 server time. The target
// build runs COMPATIBLE_MODE=0, so MySQL's UNIX_TIMESTAMP() is unavailable
// (error -2207). DATEDIFF against the UTC-extracted system timestamp was
// verified to return the correct epoch seconds independent of the session zone.
const dm8UnixTimestampSQL = "SELECT DATEDIFF(SS, TIMESTAMP'1970-01-01 00:00:00', SYS_EXTRACT_UTC(SYSTIMESTAMP))"

// dm8DatabaseTimestamp reads the current UTC Unix timestamp from DM8. It returns
// an error rather than falling back to application time so a missing/broken DM
// time path stays visible; GetDBTimestamp owns the unified fallback policy.
func dm8DatabaseTimestamp(db *gorm.DB) (int64, error) {
	var ts int64
	if err := db.Raw(dm8UnixTimestampSQL).Scan(&ts).Error; err != nil {
		return 0, err
	}
	if ts <= 0 {
		return 0, fmt.Errorf("DM8 returned an invalid database timestamp: %d", ts)
	}
	return ts, nil
}

// dm8TransactionLock is a single-row anchor used to serialize operations that DM8
// cannot guard with an advisory lock. The row is created at migration time and
// locked inside the caller's transaction with SELECT ... FOR UPDATE.
type dm8TransactionLock struct {
	Name string `gorm:"column:name;primaryKey;size:64"`
}

func (dm8TransactionLock) TableName() string { return "dm8_transaction_locks" }

// dm8EmailLockName is the fixed anchor all normalized-email writers share. DM8
// has no PostgreSQL-style advisory lock, and MySQL's next-key/gap lock on the
// email index is not available, so email uniqueness is serialized on this row.
const dm8EmailLockName = "normalized_email"

// ensureDM8TransactionLocks creates the lock table and seeds the email anchor. It
// is idempotent: a concurrent seed that hits the primary-key uniqueness is
// treated as success, and the anchor is verified to exist before returning.
func ensureDM8TransactionLocks(db *gorm.DB) error {
	if err := db.AutoMigrate(&dm8TransactionLock{}); err != nil {
		return fmt.Errorf("dm8 ensure lock table: %w", err)
	}
	var existing int64
	if err := db.Model(&dm8TransactionLock{}).Where("name = ?", dm8EmailLockName).Count(&existing).Error; err != nil {
		return fmt.Errorf("dm8 inspect email lock anchor: %w", err)
	}
	if existing == 0 {
		// A concurrent seeder may win the race; the primary-key uniqueness makes a
		// duplicate insert an expected, ignorable outcome.
		if err := db.Create(&dm8TransactionLock{Name: dm8EmailLockName}).Error; err != nil && !isDM8UniqueViolation(err) {
			return fmt.Errorf("dm8 seed email lock anchor: %w", err)
		}
	}
	var count int64
	if err := db.Model(&dm8TransactionLock{}).Where("name = ?", dm8EmailLockName).Count(&count).Error; err != nil {
		return fmt.Errorf("dm8 verify email lock anchor: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("dm8 email lock anchor missing after seeding (count=%d)", count)
	}
	return nil
}

// lockDM8NormalizedEmail takes the shared email-write lock inside tx. The anchor
// row must already exist; a missing row surfaces as ErrRecordNotFound so callers
// never proceed on an unlocked path.
func lockDM8NormalizedEmail(tx *gorm.DB) error {
	var anchor dm8TransactionLock
	return lockForUpdate(tx).
		Where("name = ?", dm8EmailLockName).
		First(&anchor).Error
}
