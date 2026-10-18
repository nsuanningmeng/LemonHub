package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestMidjourneyListsIssueCapabilitiesOnlyForAuthorizedTaskRows(t *testing.T) {
	previousDB, previousSecret := model.DB, common.MediaSigningSecret
	previousForward, previousAddress := setting.MjForwardUrlEnabled, system_setting.ServerAddress
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}))
	model.DB, common.MediaSigningSecret = db, "persisted-list-media-secret"
	setting.MjForwardUrlEnabled, system_setting.ServerAddress = true, "https://gateway.example"
	t.Cleanup(func() {
		model.DB, common.MediaSigningSecret = previousDB, previousSecret
		setting.MjForwardUrlEnabled, system_setting.ServerAddress = previousForward, previousAddress
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	tasks := []model.Midjourney{
		{Id: 9001, UserId: 1, MjId: "same-upstream-id", ImageUrl: "https://images.example/a.png"},
		{Id: 9002, UserId: 2, MjId: "same-upstream-id", ImageUrl: "https://images.example/b.png"},
	}
	require.NoError(t, db.Create(&tasks).Error)
	for _, test := range []struct {
		name    string
		handler gin.HandlerFunc
		count   int
	}{
		{"self", GetUserMidjourney, 1},
		{"admin", GetAllMidjourney, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/mj/", nil)
			c.Set("id", 1)
			test.handler(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Items []model.Midjourney `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			require.Len(t, response.Data.Items, test.count)
			for _, item := range response.Data.Items {
				link, err := url.Parse(item.ImageUrl)
				require.NoError(t, err)
				taskID, ownerID, err := service.ParseMidjourneyImageToken(item.MjId, link.Query().Get("token"))
				require.NoError(t, err)
				assert.Equal(t, item.Id, taskID)
				assert.Equal(t, item.UserId, ownerID)
				if test.name == "self" {
					assert.Equal(t, 1, ownerID)
				}
			}
		})
	}
}

func TestMediaSigningSecretCannotBeChangedViaGeneralOptionsAPI(t *testing.T) {
	previousSecret := common.MediaSigningSecret
	previousOptions := common.OptionMap
	common.MediaSigningSecret = "durable-secret"
	common.OptionMap = map[string]string{"MediaSigningSecret": "durable-secret", "SystemName": "example"}
	t.Cleanup(func() {
		common.MediaSigningSecret = previousSecret
		common.OptionMap = previousOptions
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"MediaSigningSecret","value":"replaced"}`))
	UpdateOption(c)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "durable-secret", common.MediaSigningSecret)
	assert.NotContains(t, recorder.Body.String(), "durable-secret")

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	GetOptions(c)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "MediaSigningSecret")
	assert.NotContains(t, recorder.Body.String(), "durable-secret")
	assert.Contains(t, recorder.Body.String(), "SystemName")
}
