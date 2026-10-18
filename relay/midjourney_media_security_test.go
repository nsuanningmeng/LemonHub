package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyImageRejectsUnsignedAccessAndIsolatesCollidingTaskIDs(t *testing.T) {
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	previousSecret, previousAddress := common.MediaSigningSecret, system_setting.ServerAddress
	previousFetch, previousForward := *system_setting.GetFetchSetting(), setting.MjForwardUrlEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}, &model.Channel{}))
	model.DB, common.MemoryCacheEnabled = db, false
	common.MediaSigningSecret = "persisted-media-test-secret"
	system_setting.ServerAddress = ""
	setting.MjForwardUrlEnabled = true
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{EnableSSRFProtection: true, AllowPrivateIp: true, AllowedPorts: []string{"1-65535"}}
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = previousDB, previousCache
		common.MediaSigningSecret, system_setting.ServerAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting(), setting.MjForwardUrlEnabled = previousFetch, previousForward
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("e", 100000)))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprintf(w, "image-for-%s", r.URL.Path)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Id: 7001, Key: "unused"}
	require.NoError(t, db.Create(&channel).Error)
	first := model.Midjourney{Id: 1001, UserId: 11, ChannelId: channel.Id, MjId: "collision", ImageUrl: upstream.URL + "/owner-a", Status: "SUCCESS"}
	second := model.Midjourney{Id: 1002, UserId: 22, ChannelId: channel.Id, MjId: "collision", ImageUrl: upstream.URL + "/owner-b", Status: "IN_PROGRESS"}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mj/image/:id", RelayMidjourneyImage)
	request := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder
	}

	for _, path := range []string{"/mj/image/collision", "/mj/image/collision?token=forged", "/mj/image/unknown"} {
		response := request(path)
		assert.Equal(t, http.StatusForbidden, response.Code)
	}
	assert.Zero(t, hits.Load(), "unauthorized image requests must not reach the upstream")

	for _, task := range []*model.Midjourney{&first, &second} {
		// Exercise the relay DTO serializer used by authenticated task fetches.
		converted := coverMidjourneyTaskDto(nil, task)
		response := request(converted.ImageUrl)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "image-for-"+strings.TrimPrefix(task.ImageUrl, upstream.URL), response.Body.String())
		assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "default-src 'none'; sandbox", response.Header().Get("Content-Security-Policy"))
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	}
	link, err := url.Parse(service.MidjourneyImageURL(&first))
	require.NoError(t, err)
	query := link.Query()
	query.Add("token", query.Get("token"))
	link.RawQuery = query.Encode()
	assert.Equal(t, http.StatusForbidden, request(link.String()).Code)
	assert.EqualValues(t, 2, hits.Load())

	link, err = url.Parse(service.MidjourneyImageURL(&first))
	require.NoError(t, err)
	require.NoError(t, db.Model(&first).Update("user_id", 99).Error)
	assert.Equal(t, http.StatusNotFound, request(link.String()).Code, "a capability cannot follow a task transferred to another owner")
	assert.EqualValues(t, 2, hits.Load())

	second.ImageUrl = upstream.URL + "/error"
	require.NoError(t, db.Model(&second).Update("image_url", second.ImageUrl).Error)
	response := request(service.MidjourneyImageURL(&second))
	assert.Equal(t, http.StatusBadGateway, response.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	assert.Len(t, body.Error, 64*1024)
}
