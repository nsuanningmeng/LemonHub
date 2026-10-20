package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationSettingsPreservePreferencesAndPermissions(t *testing.T) {
	for _, role := range []int{common.RoleCommonUser, common.RoleSubSiteAdmin, common.RoleAdminUser, common.RoleRootUser} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			db := setupManageUserTestDB(t)
			user := model.User{Username: "settings", Role: role, Setting: `{"language":"en","billing_preference":"wallet_only","sidebar_modules":"preserved","marketing_email_disabled":true,"record_request_body":true,"quota_warning_enabled":false,"channel_notify_enabled":false,"upstream_model_update_notify_enabled":false,"webhook_url":"https://old.example","webhook_secret":"old-secret"}`}
			require.NoError(t, db.Create(&user).Error)
			for _, sendFlags := range []bool{false, true} {
				payload := map[string]any{"notify_type": "email", "quota_warning_threshold": 0, "record_request_body": false}
				if sendFlags {
					payload["channel_notify_enabled"] = true
					payload["upstream_model_update_notify_enabled"] = true
				}
				body, err := common.Marshal(payload)
				require.NoError(t, err)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Set("id", user.Id)
				c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(string(body)))
				c.Request.Header.Set("Content-Type", "application/json")
				UpdateUserSetting(c)
				require.Contains(t, w.Body.String(), `"success":true`)
				var persisted model.User
				require.NoError(t, db.First(&persisted, user.Id).Error)
				var got dto.UserSetting
				require.NoError(t, common.UnmarshalJsonStr(persisted.Setting, &got))
				assert.Equal(t, "en", got.Language)
				assert.Equal(t, "wallet_only", got.BillingPreference)
				assert.Equal(t, "preserved", got.SidebarModules)
				assert.True(t, got.MarketingEmailDisabled)
				assert.True(t, got.RecordRequestBody)
				require.NotNil(t, got.QuotaWarningEnabled)
				assert.False(t, *got.QuotaWarningEnabled)
				require.NotNil(t, got.ChannelNotifyEnabled)
				assert.Equal(t, sendFlags && role >= common.RoleAdminUser, *got.ChannelNotifyEnabled)
				assert.Equal(t, sendFlags && role >= common.RoleAdminUser, got.UpstreamModelUpdateNotifyEnabled)
				assert.Empty(t, got.WebhookSecret)
				assert.Empty(t, got.WebhookUrl)
			}
		})
	}
}

func TestNotificationSettingsRejectInvalidWithoutOverwriting(t *testing.T) {
	for _, tc := range []struct{ name, stored, threshold string }{
		{"malformed stored settings", `{"language":"en"`, "0"},
		{"negative threshold", `{"language":"en"}`, "-1"},
		{"out of wallet range", `{"language":"en"}`, "9007199254740992"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupManageUserTestDB(t)
			user := model.User{Username: "invalid-settings", Role: common.RoleCommonUser, Setting: tc.stored}
			require.NoError(t, db.Create(&user).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", user.Id)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(`{"notify_type":"email","quota_warning_threshold":`+tc.threshold+`}`))
			c.Request.Header.Set("Content-Type", "application/json")
			UpdateUserSetting(c)
			require.Contains(t, w.Body.String(), `"success":false`)
			var persisted model.User
			require.NoError(t, db.First(&persisted, user.Id).Error)
			assert.Equal(t, tc.stored, persisted.Setting)
		})
	}
}
