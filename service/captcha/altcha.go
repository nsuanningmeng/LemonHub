package captcha

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Self-hosted ALTCHA proof-of-work channel. Challenges are stateless:
// the salt embeds an expiry and the whole challenge is HMAC-signed with a
// key derived from the app-wide CryptoSecret, so nothing is stored at issue
// time. Verification checks the signature, recomputes the hash, enforces
// expiry, and consumes each challenge exactly once to block replay (a
// published ALTCHA advisory class), via Redis when available so the
// guarantee holds across nodes.

const (
	altchaAlgorithm    = "SHA-256"
	altchaMaxNumber    = 300000
	altchaChallengeTTL = 10 * time.Minute
)

// AltchaChallenge matches the JSON shape the altcha widget expects from
// its challengeurl endpoint.
type AltchaChallenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int64  `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

type altchaPayload struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int64  `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// altchaKey derives the challenge-signing key from the app-wide CryptoSecret
// (itself sourced from CRYPTO_SECRET/SESSION_SECRET, shared across a cluster).
// Deriving instead of persisting a random secret means every node computes the
// identical key with no DB write and no cold-boot race, and challenge validity
// tracks the same lifetime as the app's other crypto.
func altchaKey() []byte {
	mac := hmac.New(sha256.New, []byte(common.CryptoSecret))
	mac.Write([]byte("altcha-pow-v1"))
	return mac.Sum(nil)
}

func CreateAltchaChallenge() (*AltchaChallenge, error) {
	saltBytes := make([]byte, 12)
	if _, err := rand.Read(saltBytes); err != nil {
		return nil, err
	}
	expires := time.Now().Add(altchaChallengeTTL).Unix()
	salt := hex.EncodeToString(saltBytes) + "?expires=" + strconv.FormatInt(expires, 10)
	n, err := rand.Int(rand.Reader, big.NewInt(altchaMaxNumber+1))
	if err != nil {
		return nil, err
	}
	challenge := altchaHash(salt, n.Int64())
	return &AltchaChallenge{
		Algorithm: altchaAlgorithm,
		Challenge: challenge,
		MaxNumber: altchaMaxNumber,
		Salt:      salt,
		Signature: altchaSign(challenge),
	}, nil
}

func verifyAltcha(token string) error {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	var p altchaPayload
	if err := common.Unmarshal(raw, &p); err != nil {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	if p.Algorithm != altchaAlgorithm || p.Number < 0 || p.Number > altchaMaxNumber {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	expiresAt, err := altchaExpiry(p.Salt)
	if err != nil || time.Now().After(expiresAt) {
		return errors.New("人机验证已过期，请刷新重试")
	}
	if altchaHash(p.Salt, p.Number) != p.Challenge {
		return errors.New("人机验证失败，请刷新重试")
	}
	if !hmac.Equal([]byte(altchaSign(p.Challenge)), []byte(p.Signature)) {
		return errors.New("人机验证失败，请刷新重试")
	}
	if !consumeAltchaChallenge(p.Challenge, expiresAt) {
		return errors.New("人机验证已失效，请刷新重试")
	}
	return nil
}

func altchaHash(salt string, number int64) string {
	sum := sha256.Sum256([]byte(salt + strconv.FormatInt(number, 10)))
	return hex.EncodeToString(sum[:])
}

func altchaSign(challenge string) string {
	mac := hmac.New(sha256.New, altchaKey())
	mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}

func altchaExpiry(salt string) (time.Time, error) {
	_, query, found := strings.Cut(salt, "?")
	if !found {
		return time.Time{}, errors.New("salt missing expiry")
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return time.Time{}, err
	}
	expires, err := strconv.ParseInt(values.Get("expires"), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(expires, 0), nil
}

var altchaUsedMutex sync.Mutex
var altchaUsed = make(map[string]time.Time)

// consumeAltchaChallenge marks a challenge as used and reports whether this
// was its first use. With Redis the guarantee is cluster-wide; the in-memory
// fallback is per-node, which is acceptable because challenges also expire.
func consumeAltchaChallenge(challenge string, expiresAt time.Time) bool {
	ttl := time.Until(expiresAt) + time.Minute
	if common.RedisEnabled && common.RDB != nil {
		ok, err := common.RDB.SetNX(context.Background(), "altcha_used:"+challenge, "1", ttl).Result()
		if err == nil {
			return ok
		}
		common.SysLog("altcha replay-store redis error, falling back to memory: " + err.Error())
	}
	altchaUsedMutex.Lock()
	defer altchaUsedMutex.Unlock()
	now := time.Now()
	for key, exp := range altchaUsed {
		if now.After(exp) {
			delete(altchaUsed, key)
		}
	}
	if _, used := altchaUsed[challenge]; used {
		return false
	}
	altchaUsed[challenge] = expiresAt.Add(time.Minute)
	return true
}
