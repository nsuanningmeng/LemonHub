package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Preserve rate-limited low-balance reminders while adding explicit switches.
func TestQuotaReminderPreferencesAndWalletRange(t *testing.T) {
	previousEnabled, previousThreshold := common.QuotaRemindEnabled, common.QuotaRemindThreshold
	t.Cleanup(func() { common.QuotaRemindEnabled, common.QuotaRemindThreshold = previousEnabled, previousThreshold })
	common.QuotaRemindThreshold = 1000
	on, off := true, false
	for _, tc := range []struct {
		name        string
		siteEnabled bool
		setting     dto.UserSetting
		remaining   int64
		want        bool
	}{
		{"default below threshold", true, dto.UserSetting{}, 999, true},
		{"exact threshold", true, dto.UserSetting{}, 1000, false},
		{"negative remaining", true, dto.UserSetting{}, -1, true},
		{"site off", false, dto.UserSetting{QuotaWarningEnabled: &on}, 1, false},
		{"user off", true, dto.UserSetting{QuotaWarningEnabled: &off}, 1, false},
		{"custom threshold", true, dto.UserSetting{QuotaWarningThreshold: 5000}, 4999, true},
		{"large wallet below", true, dto.UserSetting{QuotaWarningThreshold: float64(common.MaxWalletQuota)}, common.MaxWalletQuota - 1, true},
		{"large wallet exact", true, dto.UserSetting{QuotaWarningThreshold: float64(common.MaxWalletQuota)}, common.MaxWalletQuota, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaRemindEnabled = tc.siteEnabled
			assert.Equal(t, tc.want, quotaReminderDue(tc.setting, tc.remaining))
		})
	}
}

// Channel notices go to every enabled administrator whose channel switch is
// on, in that administrator's saved language. Until an administrator chooses,
// root receives them and other administrators do not; other users never do.
func TestChannelNoticesFollowEachAdministratorsSwitch(t *testing.T) {
	previousDB := model.DB
	previousMaster := common.IsMasterNode
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousNotifyLimit := constant.NotifyLimitCount
	previousClient, previousWorker := httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		model.DB = previousDB
		common.IsMasterNode = previousMaster
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		constant.NotifyLimitCount = previousNotifyLimit
		httpClient, system_setting.WorkerUrl = previousClient, previousWorker
		*fetch = previousFetch
	})
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.CasbinRule{}, &model.AuthzRole{}))
	model.DB = database
	common.IsMasterNode = true
	require.NoError(t, authz.Init(database))
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	constant.NotifyLimitCount = 100

	var mutex sync.Mutex
	received := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := common.DecodeJson(r.Body, &payload); err == nil {
			mutex.Lock()
			recipient := strings.TrimPrefix(r.URL.Path, "/")
			received[recipient] = append(received[recipient], payload.Type+" "+payload.Title)
			mutex.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient, system_setting.WorkerUrl = server.Client(), ""
	fetch.EnableSSRFProtection = false

	on, off := true, false
	for _, user := range []struct {
		name     string
		role     int
		status   int
		receives *bool
		language string
		siteId   int
		denyRead bool
	}{
		{"root-never-chose", common.RoleRootUser, common.UserStatusEnabled, nil, "", 0, false},
		{"root-turned-off", common.RoleRootUser, common.UserStatusEnabled, &off, "", 0, false},
		{"admin-never-chose", common.RoleAdminUser, common.UserStatusEnabled, nil, "", 0, false},
		{"admin-turned-on", common.RoleAdminUser, common.UserStatusEnabled, &on, "zh-CN", 0, false},
		{"admin-without-channel-read", common.RoleAdminUser, common.UserStatusEnabled, &on, "", 0, true},
		{"disabled-admin-turned-on", common.RoleAdminUser, common.UserStatusDisabled, &on, "", 0, false},
		{"user-with-switch-stored-on", common.RoleCommonUser, common.UserStatusEnabled, &on, "", 0, false},
		{"site-owner-with-switch", common.RoleSubSiteAdmin, common.UserStatusEnabled, &on, "", 17, false},
		{"site-admin-with-switch", common.RoleAdminUser, common.UserStatusEnabled, &on, "", 17, false},
	} {
		setting, err := common.Marshal(dto.UserSetting{
			NotifyType:                       dto.NotifyTypeWebhook,
			WebhookUrl:                       server.URL + "/" + user.name,
			ChannelNotifyEnabled:             user.receives,
			UpstreamModelUpdateNotifyEnabled: user.receives != nil && *user.receives,
			Language:                         user.language,
		})
		require.NoError(t, err)
		saved := model.User{Username: user.name, Role: user.role, Status: user.status, Setting: string(setting), SiteId: user.siteId, AffCode: user.name}
		require.NoError(t, database.Create(&saved).Error)
		if user.denyRead {
			require.NoError(t, authz.SetUserPermissions(saved.Id, authz.PermissionsMap{authz.ResourceChannel: {authz.ActionRead: false}}))
			require.False(t, authz.Can(saved.Id, saved.Role, authz.ChannelRead))
		}
	}
	channel := &model.Channel{Name: "notice-review", Key: "fixture-key", Type: 1, Status: common.ChannelStatusEnabled, Group: "default", Models: "test-model"}
	require.NoError(t, channel.Insert())

	DisableChannel(types.ChannelError{ChannelId: channel.Id, ChannelName: channel.Name, AutoBan: true}, "upstream failed")
	require.True(t, EnableChannel(channel.Id, channel.Key, channel.Name))
	NotifyChannelWatchers(dto.NotifyTypeChannelTest, func(lang string) (string, string) {
		return i18n.Translate(lang, i18n.MsgChannelTestCompletedSubject), i18n.Translate(lang, i18n.MsgChannelTestCompletedContent)
	})
	NotifyUpstreamModelUpdateWatchers("Upstream model update", "Private upstream details")

	disabledType := formatNotifyType(channel.Id, common.ChannelStatusAutoDisabled)
	enabledType := formatNotifyType(channel.Id, common.ChannelStatusEnabled)
	assert.Equal(t, map[string][]string{
		"root-never-chose": {
			fmt.Sprintf(`%s Channel "notice-review" (#%d) has been disabled`, disabledType, channel.Id),
			fmt.Sprintf(`%s Channel "notice-review" (#%d) has been enabled`, enabledType, channel.Id),
			"channel_test Channel test completed",
		},
		"admin-turned-on": {
			fmt.Sprintf("%s 通道「notice-review」（#%d）已被禁用", disabledType, channel.Id),
			fmt.Sprintf("%s 通道「notice-review」（#%d）已被启用", enabledType, channel.Id),
			"channel_test 通道测试完成",
			"channel_update Upstream model update",
		},
	}, received)
}
