package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const midjourneyMediaTTL = 24 * time.Hour

var ErrMidjourneyImageTokenInvalid = errors.New("midjourney image link is invalid or expired")

type midjourneyImageClaims struct {
	TaskID    int    `json:"task_id"`
	UserID    int    `json:"user_id"`
	MJID      string `json:"mj_id"`
	ExpiresAt int64  `json:"expires_at"`
}

// MidjourneyImageURL issues a limited media capability after the caller has
// authorized access to the task. A browser img or API client can fetch it
// without forwarding a dashboard session or relay API key into the URL.
func MidjourneyImageURL(task *model.Midjourney) string {
	if task == nil || task.Id <= 0 || task.UserId <= 0 || task.MjId == "" || task.ImageUrl == "" {
		return ""
	}
	claims := midjourneyImageClaims{
		TaskID: task.Id, UserID: task.UserId, MJID: task.MjId,
		ExpiresAt: time.Now().Add(midjourneyMediaTTL).Unix(),
	}
	payload, err := common.Marshal(claims)
	if err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac, err := midjourneyImageMAC(encoded)
	if err != nil {
		common.SysError("cannot issue media link: signing secret is not initialized")
		return ""
	}
	query := url.Values{"token": {encoded + "." + base64.RawURLEncoding.EncodeToString(mac)}}
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/mj/image/" + url.PathEscape(task.MjId) + "?" + query.Encode()
}

// ParseMidjourneyImageToken verifies the capability before any database query.
// Both the unique database row and owner are returned for an exact lookup;
// upstream MJ IDs alone are not unique across different channels or users.
func ParseMidjourneyImageToken(mjID, token string) (taskID, userID int, err error) {
	if len(token) > 4096 {
		return 0, 0, ErrMidjourneyImageTokenInvalid
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return 0, 0, ErrMidjourneyImageTokenInvalid
	}
	provided, decodeErr := base64.RawURLEncoding.DecodeString(signature)
	expected, signErr := midjourneyImageMAC(payload)
	if decodeErr != nil || signErr != nil || !hmac.Equal(provided, expected) {
		return 0, 0, ErrMidjourneyImageTokenInvalid
	}
	raw, decodeErr := base64.RawURLEncoding.DecodeString(payload)
	var claims midjourneyImageClaims
	if decodeErr != nil || common.Unmarshal(raw, &claims) != nil ||
		claims.TaskID <= 0 || claims.UserID <= 0 || claims.MJID == "" || claims.MJID != mjID || claims.ExpiresAt <= time.Now().Unix() {
		return 0, 0, ErrMidjourneyImageTokenInvalid
	}
	return claims.TaskID, claims.UserID, nil
}

func midjourneyImageMAC(payload string) ([]byte, error) {
	common.OptionMapRWMutex.RLock()
	secret := common.MediaSigningSecret
	common.OptionMapRWMutex.RUnlock()
	if secret == "" {
		return nil, ErrMidjourneyImageTokenInvalid
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("new-api/midjourney-image/v1:" + payload))
	return mac.Sum(nil), nil
}
