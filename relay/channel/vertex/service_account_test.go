package vertex

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAccessTokenFetch replaces the Google token exchange for one test and
// clears the token cache before and after it.
func stubAccessTokenFetch(t *testing.T, fetch func(Credentials, dto.ChannelSettings) (string, time.Duration, error)) {
	t.Helper()
	accessTokens.Clear()
	original := fetchAccessToken
	fetchAccessToken = fetch
	t.Cleanup(func() {
		fetchAccessToken = original
		accessTokens.Clear()
	})
}

func newAccessTokenTestInfo(channelID int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID}}
}

func newAccessTokenTestAdaptor(keyID string) *Adaptor {
	return &Adaptor{AccountCredentials: Credentials{ClientEmail: "sa@project.iam.gserviceaccount.com", PrivateKeyID: keyID}}
}

func TestGetAccessTokenReusesTokenUntilNearExpiry(t *testing.T) {
	var calls atomic.Int32
	stubAccessTokenFetch(t, func(Credentials, dto.ChannelSettings) (string, time.Duration, error) {
		return fmt.Sprintf("token-%d", calls.Add(1)), time.Hour, nil
	})
	info := newAccessTokenTestInfo(7)
	a := newAccessTokenTestAdaptor("key-1")

	first, err := getAccessToken(a, info)
	require.NoError(t, err)
	second, err := getAccessToken(a, info)
	require.NoError(t, err)
	assert.Equal(t, "token-1", first)
	assert.Equal(t, first, second)
	assert.Equal(t, int32(1), calls.Load())

	// A token inside the refresh margin is replaced.
	value, ok := accessTokens.Load("7")
	require.True(t, ok)
	entry := value.(cachedAccessToken)
	entry.expiresAt = time.Now().Add(-time.Second)
	accessTokens.Store("7", entry)

	third, err := getAccessToken(a, info)
	require.NoError(t, err)
	assert.Equal(t, "token-2", third)
	assert.Equal(t, int32(2), calls.Load())
}

func TestGetAccessTokenSeparatesKeySlotsAndReplacedKeys(t *testing.T) {
	var calls atomic.Int32
	stubAccessTokenFetch(t, func(creds Credentials, _ dto.ChannelSettings) (string, time.Duration, error) {
		calls.Add(1)
		return creds.PrivateKeyID, time.Hour, nil
	})

	multiKey := newAccessTokenTestInfo(9)
	multiKey.ChannelIsMultiKey = true
	multiKey.ChannelMultiKeyIndex = 1
	slotOne, err := getAccessToken(newAccessTokenTestAdaptor("slot-1"), multiKey)
	require.NoError(t, err)
	multiKey.ChannelMultiKeyIndex = 2
	slotTwo, err := getAccessToken(newAccessTokenTestAdaptor("slot-2"), multiKey)
	require.NoError(t, err)
	assert.Equal(t, "slot-1", slotOne)
	assert.Equal(t, "slot-2", slotTwo)

	// Replacing the channel's key fetches a token for the new key.
	single := newAccessTokenTestInfo(10)
	_, err = getAccessToken(newAccessTokenTestAdaptor("old"), single)
	require.NoError(t, err)
	replaced, err := getAccessToken(newAccessTokenTestAdaptor("new"), single)
	require.NoError(t, err)
	assert.Equal(t, "new", replaced)
	assert.Equal(t, int32(4), calls.Load())
}

func TestGetAccessTokenSharesOneExchangeAcrossConcurrentRequests(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{})
	stubAccessTokenFetch(t, func(Credentials, dto.ChannelSettings) (string, time.Duration, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return "shared", time.Hour, nil
	})
	info := newAccessTokenTestInfo(11)
	a := newAccessTokenTestAdaptor("key-1")

	const workers = 8
	var wg sync.WaitGroup
	results := make([]string, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = getAccessToken(a, info)
		}()
	}
	<-entered
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), calls.Load())
	for _, token := range results {
		assert.Equal(t, "shared", token)
	}
}

func TestGetAccessTokenDoesNotCacheFailures(t *testing.T) {
	var calls atomic.Int32
	stubAccessTokenFetch(t, func(Credentials, dto.ChannelSettings) (string, time.Duration, error) {
		if calls.Add(1) == 1 {
			return "", 0, errors.New("exchange failed")
		}
		return "recovered", time.Hour, nil
	})
	info := newAccessTokenTestInfo(12)
	a := newAccessTokenTestAdaptor("key-1")

	_, err := getAccessToken(a, info)
	require.ErrorContains(t, err, "exchange failed")
	token, err := getAccessToken(a, info)
	require.NoError(t, err)
	assert.Equal(t, "recovered", token)
}

func TestGetAccessTokenReplacedPrivateKeyInvalidatesEvenWithoutKeyID(t *testing.T) {
	var calls atomic.Int32
	stubAccessTokenFetch(t, func(creds Credentials, _ dto.ChannelSettings) (string, time.Duration, error) {
		calls.Add(1)
		return "token-" + creds.PrivateKey, time.Hour, nil
	})
	a := newAccessTokenTestAdaptor("")
	a.AccountCredentials.PrivateKey = "old-key"
	info := newAccessTokenTestInfo(13)
	first, err := getAccessToken(a, info)
	require.NoError(t, err)
	a.AccountCredentials.PrivateKey = "new-key"
	second, err := getAccessToken(a, info)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.Equal(t, int32(2), calls.Load())
}

func TestGetAccessTokenDoesNotCacheShortLivedTokens(t *testing.T) {
	var calls atomic.Int32
	stubAccessTokenFetch(t, func(Credentials, dto.ChannelSettings) (string, time.Duration, error) {
		return fmt.Sprint(calls.Add(1)), time.Minute, nil
	})
	a, info := newAccessTokenTestAdaptor("short"), newAccessTokenTestInfo(14)
	first, err := getAccessToken(a, info)
	require.NoError(t, err)
	second, err := getAccessToken(a, info)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
}

func TestAccessTokenResponseRejectsInvalidCredentialsWithoutLeakingBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       time.Duration
		fail       bool
	}{
		{"default", `{"access_token":"secret-token"}`, 200, time.Hour, false},
		{"short", `{"access_token":"secret-token","expires_in":60}`, 200, time.Minute, false},
		{"zero", `{"access_token":"secret-token","expires_in":0}`, 200, 0, false},
		{"overflow", `{"access_token":"secret-token","expires_in":9223372036854775807}`, 200, time.Hour, false},
		{"negative", `{"access_token":"secret-token","expires_in":-1}`, 200, 0, true},
		{"missing", `{"error":"secret-token"}`, 200, 0, true},
		{"empty", `{"access_token":""}`, 200, 0, true},
		{"http failure", `{"access_token":"secret-token"}`, 401, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}
			token, lifetime, err := readAccessTokenResponse(response)
			if tc.fail {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "secret-token")
				assert.Empty(t, token)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "secret-token", token)
			assert.Equal(t, tc.want, lifetime)
		})
	}
}
