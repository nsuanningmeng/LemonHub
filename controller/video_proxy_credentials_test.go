package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoProxyRestrictsProviderCredentialsToAPIOrigin(t *testing.T) {
	originalDB, originalMemoryCache := model.DB, common.MemoryCacheEnabled
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = originalDB, originalMemoryCache
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})
	common.MemoryCacheEnabled = false
	fetchSetting.EnableSSRFProtection = false // Only local HTTP fixtures are used.
	service.InitHttpClient()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))

	const credential = "private-provider-key"
	var storageHeaders http.Header
	var storageQuery url.Values
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageHeaders, storageQuery = r.Header.Clone(), r.URL.Query()
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	t.Cleanup(storage.Close)
	var providerHeaders http.Header
	var providerQuery url.Values
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerHeaders, providerQuery = r.Header.Clone(), r.URL.Query()
		if r.URL.Path == "/redirect" || r.URL.Path == "/v1/videos/upstream/content" {
			http.Redirect(w, r, storage.URL+"/video?signature=storage-signature", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	t.Cleanup(provider.Close)

	for _, tc := range []struct {
		name        string
		channelType int
		resultURL   string
		redirect    bool
		credentials bool
	}{
		{"gemini same origin", constant.ChannelTypeGemini, provider.URL + "/video", false, true},
		{"gemini signed storage", constant.ChannelTypeGemini, storage.URL + "/video?signature=storage-signature", false, false},
		{"gemini redirect", constant.ChannelTypeGemini, provider.URL + "/redirect", true, true},
		{"openai redirect", constant.ChannelTypeOpenAI, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storageHeaders, storageQuery, providerHeaders, providerQuery = nil, nil, nil, nil
			channel := model.Channel{Type: tc.channelType, Key: credential, BaseURL: &provider.URL}
			require.NoError(t, db.Create(&channel).Error)
			data, marshalErr := common.Marshal(map[string]string{"uri": tc.resultURL})
			require.NoError(t, marshalErr)
			task := model.Task{TaskID: tc.name, UserId: 7, ChannelId: channel.Id, Status: model.TaskStatusSuccess,
				Data: data, PrivateData: model.TaskPrivateData{Key: credential, UpstreamTaskID: "upstream"}}
			require.NoError(t, db.Create(&task).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/test/content", nil)
			ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			ctx.Set("id", task.UserId)
			VideoProxy(ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Equal(t, "video", recorder.Body.String())
			if tc.credentials {
				require.NotNil(t, providerHeaders)
				if tc.channelType == constant.ChannelTypeGemini {
					assert.Equal(t, credential, providerHeaders.Get("x-goog-api-key"))
				} else {
					assert.Equal(t, "Bearer "+credential, providerHeaders.Get("Authorization"))
				}
				assert.Empty(t, providerQuery.Get("key"), "do not append credentials to URLs")
			}
			if tc.redirect || !tc.credentials {
				require.NotNil(t, storageHeaders)
				assert.Empty(t, storageHeaders.Get("x-goog-api-key"))
				assert.Empty(t, storageHeaders.Get("Authorization"))
				assert.Empty(t, storageHeaders.Get("Referer"))
				assert.Empty(t, storageQuery.Get("key"))
				assert.Equal(t, "storage-signature", storageQuery.Get("signature"))
			}
		})
	}
}
