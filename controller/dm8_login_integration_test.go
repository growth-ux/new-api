package controller

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// TestDM8PasswordLoginHTTP exercises the full password-login handler on a
// case-sensitive DM8 instance. It only inserts and removes its own records.
func TestDM8PasswordLoginHTTP(t *testing.T) {
	dsn := os.Getenv("TEST_DM_DSN")
	if dsn == "" {
		t.Skip("TEST_DM_DSN not set")
	}
	expectedSchema := os.Getenv("TEST_DM_SCHEMA")
	require.True(t, strings.HasPrefix(expectedSchema, "NEWAPI_TEST_"), "TEST_DM_SCHEMA must be an isolated test schema")
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.True(t, net.ParseIP(parsed.Hostname()).IsLoopback(), "DM8 login test only permits loopback instances")

	prevDB, prevLogDB := model.DB, model.LOG_DB
	prevMain, prevLog := common.MainDatabaseType(), common.LogDatabaseType()
	prevMaster, prevRedis := common.IsMasterNode, common.RedisEnabled
	prevPasswordLogin, prevPasswordEncryption := common.PasswordLoginEnabled, common.PasswordLoginEncryptionEnabled
	t.Cleanup(func() {
		if model.DB != nil {
			if pool, err := model.DB.DB(); err == nil {
				_ = pool.Close()
			}
		}
		model.DB, model.LOG_DB = prevDB, prevLogDB
		common.SetDatabaseTypes(prevMain, prevLog)
		common.IsMasterNode, common.RedisEnabled = prevMaster, prevRedis
		common.PasswordLoginEnabled, common.PasswordLoginEncryptionEnabled = prevPasswordLogin, prevPasswordEncryption
	})
	t.Setenv("SQL_DSN", dsn)
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false // This test reads the existing migrated schema.
	common.RedisEnabled = false
	common.PasswordLoginEnabled = true
	common.PasswordLoginEncryptionEnabled = false
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	var schema string
	require.NoError(t, model.DB.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&schema).Error)
	require.Equal(t, expectedSchema, schema)
	var caseSensitive int
	require.NoError(t, model.DB.Raw("SELECT CASE_SENSITIVE()").Scan(&caseSensitive).Error)
	require.Equal(t, 1, caseSensitive)
	require.NoError(t, i18n.Init())

	password := "dm8-login-password-123"
	hash, err := common.Password2Hash(password)
	require.NoError(t, err)
	probeID := time.Now().UnixNano()
	user := &model.User{
		Username:    fmt.Sprintf("dm8http%012d", probeID%1e12),
		AffCode:     fmt.Sprintf("dm8-http-%d", probeID),
		Password:    hash,
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
		AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(user).Error)
	t.Cleanup(func() {
		_ = model.DB.Unscoped().Where(clause.Eq{Column: clause.Column{Name: "user_id"}, Value: user.Id}).Delete(&model.UserSession{}).Error
		_ = model.DB.Unscoped().Where(clause.Eq{Column: clause.Column{Name: "user_id"}, Value: user.Id}).Delete(&model.AuthFlow{}).Error
		_ = model.LOG_DB.Unscoped().Where(clause.Eq{Column: clause.Column{Name: "user_id"}, Value: user.Id}).Delete(&model.AuditLog{}).Error
		_ = model.DB.Unscoped().Delete(user).Error
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/user/login", Login)
	router.POST("/api/user/login/2fa", Verify2FALogin)
	router.GET("/api/channel", GetAllChannels)
	router.GET("/api/channel/", GetAllChannels)
	router.GET("/api/data/", GetAllQuotaDates)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, user.Username, password)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	require.True(t, response.Success, recorder.Body.String())
	activeSessions, err := model.CountActiveUserSessions(user.Id, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, activeSessions, "password login must issue a session")

	twoFA := &model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}
	require.NoError(t, model.DB.Create(twoFA).Error)
	t.Cleanup(func() { _ = model.DB.Unscoped().Delete(twoFA).Error })
	challengeRecorder := httptest.NewRecorder()
	challengeRequest := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, user.Username, password)))
	challengeRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(challengeRecorder, challengeRequest)
	var challengeResponse struct {
		Success bool `json:"success"`
		Data    struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(challengeRecorder.Body.Bytes(), &challengeResponse), challengeRecorder.Body.String())
	require.True(t, challengeResponse.Success, challengeRecorder.Body.String())
	require.NotEmpty(t, challengeResponse.Data.FlowToken, "enabled 2FA must return a login challenge")

	code, err := totp.GenerateCode(twoFA.Secret, time.Now())
	require.NoError(t, err)
	verifyRecorder := httptest.NewRecorder()
	verifyRequest := httptest.NewRequest(http.MethodPost, "/api/user/login/2fa", strings.NewReader(fmt.Sprintf(`{"flow_token":%q,"code":%q}`, challengeResponse.Data.FlowToken, code)))
	verifyRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(verifyRecorder, verifyRequest)
	var verifyResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(verifyRecorder.Body.Bytes(), &verifyResponse), verifyRecorder.Body.String())
	require.True(t, verifyResponse.Success, verifyRecorder.Body.String())
	activeSessions, err = model.CountActiveUserSessions(user.Id, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 2, activeSessions, "2FA verification must issue a session")

	t.Run("channel list", func(t *testing.T) {
		channelsRecorder := httptest.NewRecorder()
		channelsRequest := httptest.NewRequest(http.MethodGet, "/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20", nil)
		router.ServeHTTP(channelsRecorder, channelsRequest)
		var channelsResponse struct {
			Success bool `json:"success"`
			Data    struct {
				TypeCounts map[string]int64 `json:"type_counts"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(channelsRecorder.Body.Bytes(), &channelsResponse), channelsRecorder.Body.String())
		require.True(t, channelsResponse.Success, channelsRecorder.Body.String())
		require.NotNil(t, channelsResponse.Data.TypeCounts, channelsRecorder.Body.String())
	})
	t.Run("channel tag list", func(t *testing.T) {
		channelsRecorder := httptest.NewRecorder()
		channelsRequest := httptest.NewRequest(http.MethodGet, "/api/channel/?tag_mode=true&id_sort=false&p=1&page_size=20", nil)
		router.ServeHTTP(channelsRecorder, channelsRequest)
		var channelsResponse struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(channelsRecorder.Body.Bytes(), &channelsResponse), channelsRecorder.Body.String())
		require.True(t, channelsResponse.Success, channelsRecorder.Body.String())
	})
	t.Run("quota dashboard", func(t *testing.T) {
		dataRecorder := httptest.NewRecorder()
		now := time.Now().Unix()
		dataRequest := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/data/?start_timestamp=%d&end_timestamp=%d&default_time=hour", now-3600, now), nil)
		router.ServeHTTP(dataRecorder, dataRequest)
		var dataResponse struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(dataRecorder.Body.Bytes(), &dataResponse), dataRecorder.Body.String())
		require.True(t, dataResponse.Success, dataRecorder.Body.String())
	})
}
