package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealtimeTaskFetchUsesCredentialSnapshotAndRejectsUnknownMulti(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	t.Cleanup(func() { model.DB = oldDB; common.MemoryCacheEnabled = oldCache })
	common.MemoryCacheEnabled = false
	service.InitHttpClient()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	for i, tc := range []struct {
		name, key, channelKey, want string
		multi                       bool
	}{
		{"snapshot_reordered", "chosen-A", "chosen-B\nchosen-A", "chosen-A", true},
		{"legacy_single", "", "legacy-A", "legacy-A", false},
		{"legacy_multi_unknown", "", "chosen-B\nchosen-A", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			seen := ""
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				seen = r.Header.Get("x-goog-api-key")
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"name":"operations/test","done":false}`))
			}))
			defer upstream.Close()
			ch := model.Channel{Id: i + 700, Type: constant.ChannelTypeGemini, Key: tc.channelKey, BaseURL: common.GetPointer(upstream.URL), ChannelInfo: model.ChannelInfo{IsMultiKey: tc.multi}}
			require.NoError(t, db.Create(&ch).Error)
			task := model.Task{TaskID: tc.name, ChannelId: ch.Id, PrivateData: model.TaskPrivateData{Key: tc.key, UpstreamTaskID: taskcommon.EncodeLocalTaskID("operations/test")}}
			require.NoError(t, db.Create(&task).Error)
			result := tryRealtimeFetch(&task, false)
			if tc.want == "" {
				require.Zero(t, calls)
				require.Nil(t, result)
			} else {
				require.Equal(t, 1, calls)
				require.Equal(t, tc.want, seen)
				require.NotEmpty(t, result)
			}
		})
	}
}

// A continuation chooses a currently enabled key on the origin channel, then
// snapshots that selection. The initially distributed channel is unrelated.
func TestResolveOriginTaskSnapshotsReboundChannelCredentialProvenance(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	t.Cleanup(func() { model.DB = oldDB; common.MemoryCacheEnabled = oldCache })
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	for i, tc := range []struct {
		name                       string
		previousMulti, originMulti bool
		originKeys, wantKey        string
		wantIndex                  int
		statuses                   map[int]int
	}{
		{"single_to_multi_index_zero", false, true, "selected-A\ndisabled-B", "selected-A", 0, map[int]int{1: common.ChannelStatusManuallyDisabled}},
		{"multi_to_single", true, false, "selected-single", "selected-single", 0, nil},
		{"multi_to_multi_index_one", true, true, "disabled-A\nselected-B", "selected-B", 1, map[int]int{0: common.ChannelStatusManuallyDisabled}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := model.Channel{Id: i + 1800, Type: constant.ChannelTypeSora, Status: common.ChannelStatusEnabled, Key: tc.originKeys, ChannelInfo: model.ChannelInfo{IsMultiKey: tc.originMulti, MultiKeyMode: constant.MultiKeyModeRandom, MultiKeyStatusList: tc.statuses}}
			require.NoError(t, db.Create(&ch).Error)
			origin := model.Task{TaskID: "origin-" + tc.name, UserId: 7, ChannelId: ch.Id, PrivateData: model.TaskPrivateData{Key: "historical-origin-key"}}
			require.NoError(t, db.Create(&origin).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/tasks/continue", nil)
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, tc.previousMulti)
			common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 7)
			info := &relaycommon.RelayInfo{
				UserId:        7,
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: ch.Id + 100, ApiKey: "previous-distribution-key", ChannelIsMultiKey: tc.previousMulti, ChannelMultiKeyIndex: 7},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{OriginTaskID: origin.TaskID},
			}
			require.Nil(t, ResolveOriginTask(c, info))
			require.Equal(t, ch.Id, info.ChannelId)
			require.Equal(t, tc.wantKey, info.ApiKey)
			require.Equal(t, tc.originMulti, info.ChannelIsMultiKey)
			require.Equal(t, tc.wantIndex, info.ChannelMultiKeyIndex)
			// RelayTaskSubmit reconstructs this metadata from context before submit.
			info.InitChannelMeta(c)
			require.Equal(t, tc.wantKey, info.ApiKey)
			require.Equal(t, tc.originMulti, info.ChannelIsMultiKey)
			require.Equal(t, tc.wantIndex, info.ChannelMultiKeyIndex)
			created := model.InitTask(constant.TaskPlatform("sora"), info)
			require.NoError(t, db.Create(created).Error)
			var stored model.Task
			require.NoError(t, db.First(&stored, created.ID).Error)
			require.Equal(t, ch.Id, stored.ChannelId)
			require.Equal(t, tc.wantKey, stored.PrivateData.Key)
			require.NotNil(t, stored.PrivateData.IsMultiKey)
			require.Equal(t, tc.originMulti, *stored.PrivateData.IsMultiKey)
			if tc.originMulti {
				require.NotNil(t, stored.PrivateData.KeyIndex)
				require.Equal(t, tc.wantIndex, *stored.PrivateData.KeyIndex)
			} else {
				require.Nil(t, stored.PrivateData.KeyIndex)
			}
		})
	}
}
