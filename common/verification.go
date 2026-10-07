package common

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

var ErrVerificationStoreUnavailable = errors.New("verification service unavailable")

var consumeVerificationCode = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  redis.call("DEL", KEYS[1])
  return 1
end
return 0
`)

// Length-prefixing separates purpose and identity without ambiguous concatenation;
// hashing keeps email addresses out of Redis key names.
func verificationStorageKey(key, purpose string) string {
	identity := fmt.Sprintf("%d:%s%s", len(purpose), purpose, key)
	return fmt.Sprintf("verification:v1:%x", sha256.Sum256([]byte(identity)))
}

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) error {
	if code == "" || VerificationValidMinutes <= 0 {
		return ErrVerificationStoreUnavailable
	}
	storageKey := verificationStorageKey(key, purpose)
	if RedisEnabled {
		if RDB == nil {
			return ErrVerificationStoreUnavailable
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := RDB.Set(ctx, storageKey, code, time.Duration(VerificationValidMinutes)*time.Minute).Err(); err != nil {
			return ErrVerificationStoreUnavailable
		}
		return nil
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[storageKey] = verificationValue{
		code: code,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
	return nil
}

// VerifyCodeWithKey atomically consumes a matching, unexpired code. A storage
// failure is never a reason to fall back to another node's local state.
func VerifyCodeWithKey(key string, code string, purpose string) (bool, error) {
	if code == "" {
		return false, nil
	}
	storageKey := verificationStorageKey(key, purpose)
	if RedisEnabled {
		if RDB == nil {
			return false, ErrVerificationStoreUnavailable
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		consumed, err := consumeVerificationCode.Run(ctx, RDB, []string{storageKey}, code).Int()
		if err != nil {
			return false, ErrVerificationStoreUnavailable
		}
		return consumed == 1, nil
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[storageKey]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		delete(verificationMap, storageKey)
		return false, nil
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(value.code)) != 1 {
		return false, nil
	}
	delete(verificationMap, storageKey)
	return true, nil
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
