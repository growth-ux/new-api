package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserVerificationState is an authoritative, credential-free projection for
// choosing an authentication method. Loading it never fetches credential secrets.
type UserVerificationState struct {
	UserID      int
	Status      int
	Role        int
	AuthVersion int64
	HasPassword bool
	HasTwoFA    bool
	TwoFALocked bool
	HasPasskey  bool
}

func GetUserVerificationState(userID int) (*UserVerificationState, error) {
	return getUserVerificationState(DB, userID, false)
}

func getUserVerificationState(tx *gorm.DB, userID int, forUpdate bool) (*UserVerificationState, error) {
	if userID <= 0 {
		return nil, ErrUserSessionInvalid
	}
	var state UserVerificationState
	column := func(name string) string { return quoteModelIdentifier(tx, name) }
	// DM8 cannot alias a bare EXISTS predicate ("EXISTS (?) AS x" fails near AS);
	// wrap it in CASE so the projected column is a plain integer, matching how the
	// other dialects already project has_password.
	existsProjection := "EXISTS (?) AS %s"
	if common.UsingMainDatabase(common.DatabaseTypeDM) {
		existsProjection = "CASE WHEN EXISTS (?) THEN 1 ELSE 0 END AS %s"
	}
	selectSQL := fmt.Sprintf("%s AS %s, %s, %s, %s, CASE WHEN %s <> '' THEN 1 ELSE 0 END AS %s, ",
		column("id"), column("user_id"), column("status"), column("role"), column("auth_version"), column("password"), column("has_password")) +
		fmt.Sprintf(existsProjection, column("has_two_fa")) + ", " +
		fmt.Sprintf(existsProjection, column("two_fa_locked")) + ", " +
		fmt.Sprintf(existsProjection, column("has_passkey"))
	query := tx.Model(&User{}).Select(
		selectSQL,
		tx.Model(&TwoFA{}).Select("1").Where(column("user_id")+" = ? AND "+column("is_enabled")+" = ?", userID, true),
		tx.Model(&TwoFA{}).Select("1").Where(column("user_id")+" = ? AND "+column("is_enabled")+" = ? AND "+column("locked_until")+" > ?", userID, true, time.Now()),
		tx.Model(&PasskeyCredential{}).Select("1").Where(clause.Eq{Column: clause.Column{Name: "user_id"}, Value: userID}),
	).Where(clause.Eq{Column: clause.Column{Name: "id"}, Value: userID})
	if forUpdate {
		query = lockForUpdate(query)
	}
	if err := query.Take(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// CreateUserSessionFromLoginFlow commits the one-time login authorization and
// the resulting session together. The user lock serializes credential changes
// and session issuance, including the per-user session limits. validate runs
// inside the transaction and may write through tx.
func CreateUserSessionFromLoginFlow(token string, session *UserSession, validate func(*gorm.DB, *AuthFlow, *UserVerificationState) error) error {
	if session == nil || validate == nil {
		return ErrUserSessionInvalid
	}
	cacheDeadline := userSessionCacheDeadline()
	_, err := ConsumeAuthFlowWithAction(token, AuthFlowMatch{
		Purpose: AuthFlowPurposeLoginVerification, UserId: session.UserID,
	}, func(tx *gorm.DB, flow *AuthFlow) error {
		state, err := getUserVerificationState(tx, session.UserID, true)
		if err != nil {
			return err
		}
		if state.Status != common.UserStatusEnabled || state.AuthVersion != session.UserAuthVersion {
			return ErrUserSessionInactive
		}
		if err := validate(tx, flow, state); err != nil {
			return err
		}
		now := time.Now().Unix()
		var activeCount, issuanceCount int64
		if err := tx.Model(&UserSession{}).Where(quoteModelIdentifier(tx, "user_id")+" = ? AND "+quoteModelIdentifier(tx, "status")+" = ? AND "+quoteModelIdentifier(tx, "expires_at")+" > ?", session.UserID, UserSessionStatusActive, now).Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount >= int64(common.UserSessionActiveLimit) {
			return ErrUserSessionLimit
		}
		if err := tx.Model(&UserSession{}).Where(quoteModelIdentifier(tx, "user_id")+" = ? AND "+quoteModelIdentifier(tx, "created_at")+" > ?", session.UserID, now-common.UserSessionIssuanceWindowSeconds).Count(&issuanceCount).Error; err != nil {
			return err
		}
		if issuanceCount >= int64(common.UserSessionIssuanceLimit) {
			return ErrUserSessionIssuanceLimit
		}
		return createUserSessionWithTx(tx, session)
	})
	if err != nil {
		return err
	}
	return publishCreatedUserSession(session, cacheDeadline)
}
