package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGitHubLegacyIdentityMigrationLogDoesNotExposeProviderSubjects(t *testing.T) {
	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ExternalIdentityClaim{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
	})

	const (
		legacyID = "legacy-github-login-pii-marker"
		newID    = "new-github-numeric-subject-marker"
	)
	legacyUser := model.User{
		Username: "github-legacy-log-user",
		Password: "password",
		AffCode:  "github-legacy-log-user",
		GitHubId: legacyID,
		SiteId:   0,
	}
	require.NoError(t, db.Create(&legacyUser).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/github", nil)
	var migrated *model.User
	logs := capturePaymentWebhookLogs(func() {
		migrated, err = findOrCreateOAuthUser(ctx, &oauth.GitHubProvider{}, &oauth.OAuthUser{
			ProviderUserID: newID,
			Extra:          map[string]any{"legacy_id": legacyID},
		}, "")
	})

	require.NoError(t, err)
	require.NotNil(t, migrated)
	assert.Equal(t, legacyUser.Id, migrated.Id)
	assert.Equal(t, newID, migrated.GitHubId)
	assert.Contains(t, logs, "legacy identity migration started")
	assert.NotContains(t, logs, legacyID)
	assert.NotContains(t, logs, newID)
}
