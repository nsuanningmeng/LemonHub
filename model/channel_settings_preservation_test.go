package model

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelSettingsReadFailurePreservesObjectAndStorage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "channels.db")), newGormConfig(false))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		require.NoError(t, sqlDB.Close())
	})
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
	})
	for _, fixture := range []struct{ name, setting, settings string }{
		{
			name:     "invalid syntax",
			setting:  `{"proxy":"https://syntax-secret@proxy.example",`,
			settings: `{"advanced_custom":{"private":"syntax-other-secret"},`,
		},
		{
			name:     "type error after partially decoded values",
			setting:  `{"proxy":"https://partial-secret@proxy.example","force_format":"invalid"}`,
			settings: `{"disable_store":true,"vertex_key_type":{"private":"partial-other-secret"}}`,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			channel := Channel{Name: "database name", Key: "credential-sentinel", Setting: &fixture.setting, OtherSettings: fixture.settings}
			require.NoError(t, db.Create(&channel).Error)
			channel.Name = "stale in-memory name"
			original := channel
			for i := 0; i < 2; i++ {
				assert.Equal(t, dto.ChannelSettings{}, channel.GetSetting())
				assert.Equal(t, dto.ChannelOtherSettings{}, channel.GetOtherSettings())
				assert.Equal(t, original, channel, "a getter must not mutate its caller's original settings")
			}
			var stored Channel
			require.NoError(t, db.First(&stored, channel.Id).Error)
			assert.Equal(t, "database name", stored.Name, "reading invalid settings must not save a stale channel snapshot")
			require.NotNil(t, stored.Setting)
			assert.Equal(t, fixture.setting, *stored.Setting)
			assert.Equal(t, fixture.settings, stored.OtherSettings)
			assert.Equal(t, "credential-sentinel", stored.Key)
		})
	}
	assert.Contains(t, logs.String(), "original value preserved")
	for _, secret := range []string{"syntax-secret", "syntax-other-secret", "partial-secret", "partial-other-secret", "credential-sentinel"} {
		assert.NotContains(t, logs.String(), secret)
	}
}
