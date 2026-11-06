package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetStatusUptimeVisibility(t *testing.T) {
	settings := console_setting.GetConsoleSetting()
	original := *settings
	originalOptions := common.OptionMap
	t.Cleanup(func() { *settings = original; common.OptionMap = originalOptions })
	common.OptionMap = map[string]string{}
	for _, tt := range []struct {
		name    string
		enabled bool
		groups  string
		want    bool
	}{
		{name: "unconfigured monitor stays hidden", enabled: true, groups: "", want: false},
		{name: "empty monitor list stays hidden", enabled: true, groups: "[]", want: false},
		{name: "configured monitor is visible", enabled: true, groups: `[{"name":"Services","url":"https://status.example.com","slug":"services"}]`, want: true},
		{name: "disabled monitor stays hidden", enabled: false, groups: `[{"name":"Services"}]`, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings.UptimeKumaEnabled = tt.enabled
			settings.UptimeKumaGroups = tt.groups
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
			GetStatus(context)
			var payload struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success)
			require.Equal(t, tt.want, payload.Data["uptime_kuma_enabled"])
		})
	}
}
