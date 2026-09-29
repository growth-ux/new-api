package model

import (
	"testing"

	dm8driver "github.com/godoes/gorm-dameng/dm8"
)

func TestDM8UniqueViolationRecognizesBothServerCodes(t *testing.T) {
	for _, code := range []int32{-6602, -6625} {
		if !isDM8UniqueViolation(&dm8driver.DmError{ErrCode: code}) {
			t.Errorf("DM8 unique constraint error %d must trigger conflict handling", code)
		}
	}
	if isDM8UniqueViolation(&dm8driver.DmError{ErrCode: -6603}) {
		t.Error("unrelated DM8 error must not trigger conflict handling")
	}
}
