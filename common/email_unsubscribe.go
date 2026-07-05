package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Marketing one-click unsubscribe tokens (RFC 8058). The token is embedded in
// every bulk email's List-Unsubscribe header and footer link, and must stay
// valid across restarts and nodes — it is therefore signed with the persisted
// UnsubscribeSecret option, NOT the per-boot random SessionSecret/CryptoSecret.

const unsubscribeTokenContext = "marketing-unsub.v1."

func unsubscribeMAC(userId int) ([]byte, error) {
	if UnsubscribeSecret == "" {
		return nil, errors.New("unsubscribe secret not initialized")
	}
	mac := hmac.New(sha256.New, []byte(UnsubscribeSecret))
	mac.Write([]byte(unsubscribeTokenContext + strconv.Itoa(userId)))
	return mac.Sum(nil), nil
}

// GenerateUnsubscribeToken returns an opaque URL-safe token identifying the
// user for unauthenticated one-click unsubscribe.
func GenerateUnsubscribeToken(userId int) (string, error) {
	if userId <= 0 {
		return "", errors.New("invalid user id")
	}
	sum, err := unsubscribeMAC(userId)
	if err != nil {
		return "", err
	}
	raw := fmt.Sprintf("%d.%s", userId, hex.EncodeToString(sum))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

// ParseUnsubscribeToken verifies a token and returns the user id it was issued
// for. Tampered, malformed, or foreign-secret tokens are rejected.
func ParseUnsubscribeToken(token string) (int, error) {
	rawBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return 0, errors.New("invalid unsubscribe token")
	}
	idPart, sigPart, found := strings.Cut(string(rawBytes), ".")
	if !found {
		return 0, errors.New("invalid unsubscribe token")
	}
	userId, err := strconv.Atoi(idPart)
	if err != nil || userId <= 0 {
		return 0, errors.New("invalid unsubscribe token")
	}
	gotSig, err := hex.DecodeString(sigPart)
	if err != nil {
		return 0, errors.New("invalid unsubscribe token")
	}
	wantSig, err := unsubscribeMAC(userId)
	if err != nil {
		return 0, err
	}
	if !hmac.Equal(gotSig, wantSig) {
		return 0, errors.New("invalid unsubscribe token")
	}
	return userId, nil
}
