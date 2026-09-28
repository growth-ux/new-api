package model

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	dm8driver "github.com/godoes/gorm-dameng/dm8"
	"gorm.io/gorm"
)

// dm8UniqueViolationCode is the DM8 server error code for a uniqueness
// constraint violation, verified against the target build (-6625). It is used to
// distinguish an expected conflict from other failures in the DM create adapter.
const dm8UniqueViolationCode = int32(-6625)

// openDM8 opens a DM8 connection through the wrapped community dialector. It
// verifies server compatibility before returning and closes the pool on failure.
// The raw DSN (which carries credentials) is never logged; only the resolved
// schema name is reported.
func openDM8(dsn string) (*gorm.DB, error) {
	if !strings.HasPrefix(dsn, "dm://") {
		return nil, errors.New("invalid DM8 DSN: expected a dm:// scheme")
	}
	db, err := gorm.Open(newDM8Dialector(dsn), newGormConfig(false))
	if err != nil {
		return nil, fmt.Errorf("open DM8 connection: %w; network diagnosis: %s", sanitizeDM8ErrorOrSelf(err), dm8NetworkDiagnosis(dsn))
	}
	if err := registerDM8Create(db); err != nil {
		if pool, poolErr := db.DB(); poolErr == nil {
			_ = pool.Close()
		}
		return nil, err
	}
	if err := checkDM8Compatibility(db); err != nil {
		if pool, poolErr := db.DB(); poolErr == nil {
			_ = pool.Close()
		}
		return nil, err
	}
	return db, nil
}

// dm8NetworkDiagnosis performs a bare TCP dial to the DSN host:port so a
// communication error (e.g. dm error 6001) reports whether the network path is
// refused, timed out, or reachable. It only exposes host:port and the dial
// error, never credentials or query parameters.
func dm8NetworkDiagnosis(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" {
		return "unavailable (cannot parse host from DSN)"
	}
	conn, err := net.DialTimeout("tcp", parsed.Host, 5*time.Second)
	if err != nil {
		return "tcp dial " + parsed.Host + " failed: " + err.Error()
	}
	_ = conn.Close()
	return "tcp dial " + parsed.Host + " ok (network reachable; failure is at DM protocol/auth layer)"
}

// checkDM8Compatibility confirms the connection is live and resolves the current
// schema. It does not mutate any instance parameter; an unreachable server or an
// empty schema is a hard startup failure so DM8 never silently degrades.
func checkDM8Compatibility(db *gorm.DB) error {
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("DM8 connection pool unavailable: %w", sanitizeDM8ErrorOrSelf(err))
	}
	if err := pool.Ping(); err != nil {
		return fmt.Errorf("DM8 ping failed: %w", sanitizeDM8ErrorOrSelf(err))
	}
	var schema string
	if err := db.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&schema).Error; err != nil {
		return fmt.Errorf("DM8 read current schema failed: %w", sanitizeDM8ErrorOrSelf(err))
	}
	if strings.TrimSpace(schema) == "" {
		return errors.New("DM8 current schema is empty; check the connection schema option")
	}
	// The community migrator reads SYS.SYSOBJECTS to inspect tables and indexes.
	// Its HasTable method discards query errors, so without catalog access a
	// second startup can attempt to create already existing tables.
	var catalogRows int64
	if err := db.Raw("SELECT COUNT(*) FROM SYS.SYSOBJECTS WHERE ROWNUM <= 1").Scan(&catalogRows).Error; err != nil {
		return fmt.Errorf("DM8 migration metadata unavailable: current user must be able to read SYS.SYSOBJECTS (for example via SELECT ANY DICTIONARY): %w", sanitizeDM8ErrorOrSelf(err))
	}
	common.SysLog("using DM8 as database, current schema: " + schema)
	return nil
}

// dm8ErrorOK reports whether err (or something it wraps) is a DM8 server error
// and returns its numeric code. Control errors such as context cancellation are
// not DM8 server errors and stay untouched so errors.Is keeps working.
func dm8ErrorCode(err error) (int32, bool) {
	var dmErr *dm8driver.DmError
	if errors.As(err, &dmErr) {
		return dmErr.ErrCode, true
	}
	return 0, false
}

// sanitizeDM8Error collapses a DM8 server error to its numeric code so inlined
// data values never reach the logs. It reports whether err was a DM8 server
// error; non-DM8 errors are returned unchanged for the caller to handle.
func sanitizeDM8Error(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	if code, ok := dm8ErrorCode(err); ok {
		return fmt.Errorf("dm error %d", code), true
	}
	return err, false
}

func sanitizeDM8ErrorOrSelf(err error) error {
	if sanitized, ok := sanitizeDM8Error(err); ok {
		return sanitized
	}
	return err
}

// isDM8UniqueViolation reports whether err is a DM8 uniqueness constraint
// violation, used by the DM create adapter to honor OnConflict DoNothing.
func isDM8UniqueViolation(err error) bool {
	code, ok := dm8ErrorCode(err)
	return ok && code == dm8UniqueViolationCode
}
