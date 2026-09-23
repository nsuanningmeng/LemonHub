package controller

import (
	"net/http"
	"net/http/httptest"
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

func TestVideoProxyKeepsAuthenticatedContentPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMemoryCache := common.MemoryCacheEnabled
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCache
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})
	common.MemoryCacheEnabled = false
	// The only upstream is this test's loopback HTTP server.
	fetchSetting.EnableSSRFProtection = false
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("CDN-Cache-Control", "public, s-maxage=86400")
		w.Header().Set("Cloudflare-CDN-Cache-Control", "public, max-age=86400")
		w.Header().Set("Surrogate-Control", "max-age=86400")
		w.Header().Set("Expires", "Wed, 23 Sep 2037 12:00:00 GMT")
		w.Header().Set("Age", "42")
		w.Header().Set("ETag", `"upstream-video"`)
		w.Header().Set("Last-Modified", "Tue, 22 Sep 2026 12:00:00 GMT")
		_, _ = w.Write([]byte("video"))
	}))
	t.Cleanup(upstream.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	channel := model.Channel{Id: 1, Type: constant.ChannelTypeCustom}
	require.NoError(t, db.Create(&channel).Error)

	for _, tc := range []struct {
		name string
		url  string
	}{
		{name: "upstream", url: upstream.URL},
		{name: "data URL", url: "data:video/mp4;base64,dmlkZW8="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := model.Task{
				TaskID: tc.name, UserId: 7, ChannelId: channel.Id,
				Status:      model.TaskStatusSuccess,
				PrivateData: model.TaskPrivateData{ResultURL: tc.url},
			}
			require.NoError(t, db.Create(&task).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/test/content", nil)
			ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			ctx.Set("id", task.UserId)
			VideoProxy(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "video", recorder.Body.String())
			assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			for _, header := range []string{"CDN-Cache-Control", "Cloudflare-CDN-Cache-Control", "Surrogate-Control", "Expires", "Age", "ETag", "Last-Modified"} {
				assert.Empty(t, recorder.Header().Get(header), header)
			}
		})
	}
}
