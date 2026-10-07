package common

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Separate test processes have independent verification maps and Redis clients,
// matching the deployment in which the sending and receiving requests land on
// different nodes.
func TestVerificationCodeCrossNode(t *testing.T) {
	server := miniredis.RunT(t)
	for _, operation := range []string{"register", "verify"} {
		command := exec.Command(os.Args[0], "-test.run=^TestVerificationNodeProcess$")
		command.Env = append(os.Environ(), "VERIFICATION_TEST_NODE="+operation, "VERIFICATION_TEST_REDIS="+server.Addr())
		output, err := command.CombinedOutput()
		require.NoError(t, err, "node %s: %s", operation, output)
	}
}

func TestVerificationNodeProcess(t *testing.T) {
	operation := os.Getenv("VERIFICATION_TEST_NODE")
	if operation == "" {
		t.Skip("subprocess fixture")
	}
	RedisEnabled = true
	RDB = redis.NewClient(&redis.Options{Addr: os.Getenv("VERIFICATION_TEST_REDIS"), MaxRetries: -1})
	t.Cleanup(func() { RDB.Close() })
	if operation == "register" {
		require.NoError(t, RegisterVerificationCodeWithKey("site7:user@example.test", "654321", EmailVerificationPurpose))
		return
	}
	verified, err := VerifyCodeWithKey("site7:user@example.test", "654321", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.True(t, verified)
}

func TestVerificationCodeCanOnlyBeConsumedOnce(t *testing.T) {
	previous := RedisEnabled
	RedisEnabled = false
	t.Cleanup(func() { RedisEnabled = previous })
	require.NoError(t, RegisterVerificationCodeWithKey(t.Name(), "654321", EmailVerificationPurpose))
	verified, err := VerifyCodeWithKey(t.Name(), "654321", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.True(t, verified)
	verified, err = VerifyCodeWithKey(t.Name(), "654321", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified)
}

func setupVerificationRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previousClient, previousEnabled := RDB, RedisEnabled
	RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	RedisEnabled = true
	t.Cleanup(func() { RDB.Close(); RDB, RedisEnabled = previousClient, previousEnabled })
	return server
}

func TestVerificationCodeScopeExpiryAndReissue(t *testing.T) {
	server := setupVerificationRedis(t)
	key := "7:user@example.test"
	require.NoError(t, RegisterVerificationCodeWithKey(key, "654321", EmailVerificationPurpose))
	for _, mismatch := range []struct{ key, code, purpose string }{
		{key, "wrong", EmailVerificationPurpose},
		{key, "654321", PasswordResetPurpose},
		{"8:user@example.test", "654321", EmailVerificationPurpose},
		{"7:other@example.test", "654321", EmailVerificationPurpose},
	} {
		verified, err := VerifyCodeWithKey(mismatch.key, mismatch.code, mismatch.purpose)
		require.NoError(t, err)
		assert.False(t, verified)
	}
	verified, err := VerifyCodeWithKey(key, "654321", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.True(t, verified, "wrong codes and scope mismatches must not consume the valid code")
	require.NoError(t, RegisterVerificationCodeWithKey(key, "expired", EmailVerificationPurpose))
	server.FastForward(time.Duration(VerificationValidMinutes) * time.Minute)
	verified, err = VerifyCodeWithKey(key, "expired", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified)
	require.NoError(t, RegisterVerificationCodeWithKey(key, "old", EmailVerificationPurpose))
	require.NoError(t, RegisterVerificationCodeWithKey(key, "new", EmailVerificationPurpose))
	verified, err = VerifyCodeWithKey(key, "old", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified, "cleanup after an older send must preserve a newer code")
	verified, err = VerifyCodeWithKey(key, "new", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.True(t, verified)
	// The old purpose+key concatenation allowed unrelated identities to collide.
	require.NoError(t, RegisterVerificationCodeWithKey("bc", "first", "a"))
	require.NoError(t, RegisterVerificationCodeWithKey("c", "second", "ab"))
	verified, err = VerifyCodeWithKey("bc", "first", "a")
	require.NoError(t, err)
	assert.True(t, verified)
}

func TestVerificationCodeConcurrentConsumption(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "redis"}[useRedis], func(t *testing.T) {
			if useRedis {
				setupVerificationRedis(t)
			} else {
				previous := RedisEnabled
				RedisEnabled = false
				t.Cleanup(func() { RedisEnabled = previous })
			}
			require.NoError(t, RegisterVerificationCodeWithKey(t.Name(), "code", EmailVerificationPurpose))
			type result struct {
				verified bool
				err      error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			var ready sync.WaitGroup
			ready.Add(2)
			for range 2 {
				go func() {
					ready.Done()
					<-start
					verified, err := VerifyCodeWithKey(t.Name(), "code", EmailVerificationPurpose)
					results <- result{verified, err}
				}()
			}
			ready.Wait()
			close(start)
			consumed := 0
			for range 2 {
				result := <-results
				require.NoError(t, result.err)
				if result.verified {
					consumed++
				}
			}
			assert.Equal(t, 1, consumed)
		})
	}
}

func TestVerificationCodeStoreFailureNeverUsesLocalState(t *testing.T) {
	server := setupVerificationRedis(t)
	RedisEnabled = false
	require.NoError(t, RegisterVerificationCodeWithKey(t.Name(), "local", EmailVerificationPurpose))
	RedisEnabled = true
	server.SetError("ERR forced outage")
	require.ErrorIs(t, RegisterVerificationCodeWithKey(t.Name(), "new", EmailVerificationPurpose), ErrVerificationStoreUnavailable)
	verified, err := VerifyCodeWithKey(t.Name(), "local", EmailVerificationPurpose)
	require.ErrorIs(t, err, ErrVerificationStoreUnavailable)
	assert.False(t, verified)
	server.SetError("")
	verified, err = VerifyCodeWithKey(t.Name(), "new", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified, "failed registration must not be represented by local state")
}

func TestVerificationCodeLocalExpiry(t *testing.T) {
	previous := RedisEnabled
	RedisEnabled = false
	t.Cleanup(func() { RedisEnabled = previous })
	require.NoError(t, RegisterVerificationCodeWithKey(t.Name(), "old", EmailVerificationPurpose))
	verificationMutex.Lock()
	storageKey := verificationStorageKey(t.Name(), EmailVerificationPurpose)
	value := verificationMap[storageKey]
	value.time = time.Now().Add(-time.Duration(VerificationValidMinutes) * time.Minute)
	verificationMap[storageKey] = value
	verificationMutex.Unlock()
	verified, err := VerifyCodeWithKey(t.Name(), "old", EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified)
}
