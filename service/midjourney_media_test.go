package service

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyImageCapabilityBindsTaskOwnerAndExpires(t *testing.T) {
	previousSecret, previousAddress := common.MediaSigningSecret, system_setting.ServerAddress
	common.MediaSigningSecret = "persistent-media-secret"
	system_setting.ServerAddress = "https://gateway.example/"
	t.Cleanup(func() {
		common.MediaSigningSecret, system_setting.ServerAddress = previousSecret, previousAddress
	})
	task := &model.Midjourney{Id: 51, UserId: 7, MjId: "upstream-shared-id", ImageUrl: "https://images.example/a.png"}
	link, err := url.Parse(MidjourneyImageURL(task))
	require.NoError(t, err)
	assert.Equal(t, "https://gateway.example/mj/image/upstream-shared-id", strings.Split(link.String(), "?")[0])
	token := link.Query().Get("token")
	require.NotEmpty(t, token)
	taskID, userID, err := ParseMidjourneyImageToken(task.MjId, token)
	require.NoError(t, err)
	assert.Equal(t, task.Id, taskID)
	assert.Equal(t, task.UserId, userID)

	payload, signature, ok := strings.Cut(token, ".")
	require.True(t, ok)
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	require.NoError(t, err)
	var claims midjourneyImageClaims
	require.NoError(t, common.Unmarshal(raw, &claims))
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), claims.ExpiresAt, 2)
	claims.UserID++
	forged, err := common.Marshal(claims)
	require.NoError(t, err)
	_, _, err = ParseMidjourneyImageToken(task.MjId, base64.RawURLEncoding.EncodeToString(forged)+"."+signature)
	assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)
	_, _, err = ParseMidjourneyImageToken("other-task", token)
	assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)

	claims.UserID = task.UserId
	claims.ExpiresAt = time.Now().Add(-time.Second).Unix()
	raw, err = common.Marshal(claims)
	require.NoError(t, err)
	payload = base64.RawURLEncoding.EncodeToString(raw)
	sum, err := midjourneyImageMAC(payload)
	require.NoError(t, err)
	_, _, err = ParseMidjourneyImageToken(task.MjId, payload+"."+base64.RawURLEncoding.EncodeToString(sum))
	assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)

	for _, invalid := range []string{"", "random", token + ".extra", strings.Repeat("x", 4097)} {
		_, _, err = ParseMidjourneyImageToken(task.MjId, invalid)
		assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)
	}
	common.MediaSigningSecret = "other-installation-secret"
	_, _, err = ParseMidjourneyImageToken(task.MjId, token)
	assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)
	common.MediaSigningSecret = ""
	assert.Empty(t, MidjourneyImageURL(task))
	_, _, err = ParseMidjourneyImageToken(task.MjId, token)
	assert.ErrorIs(t, err, ErrMidjourneyImageTokenInvalid)
}
