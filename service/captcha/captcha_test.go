package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFallsBackToTurnstileForUnknownValues(t *testing.T) {
	t.Cleanup(func() { common.CaptchaProvider = "turnstile" })
	for _, value := range []string{"", "turnstile", "no-such-provider"} {
		common.CaptchaProvider = value
		assert.Equal(t, ProviderTurnstile, Provider(), "value %q", value)
	}
	common.CaptchaProvider = "geetest"
	assert.Equal(t, ProviderGeetest, Provider())
}

// --- ALTCHA -----------------------------------------------------------------

func solveAltcha(t *testing.T, ch *AltchaChallenge) string {
	t.Helper()
	for n := int64(0); n <= ch.MaxNumber; n++ {
		if altchaHash(ch.Salt, n) == ch.Challenge {
			payload, err := common.Marshal(map[string]interface{}{
				"algorithm": ch.Algorithm,
				"challenge": ch.Challenge,
				"number":    n,
				"salt":      ch.Salt,
				"signature": ch.Signature,
			})
			require.NoError(t, err)
			return base64.StdEncoding.EncodeToString(payload)
		}
	}
	t.Fatal("challenge not solvable within maxnumber")
	return ""
}

func TestAltchaRoundTripAndReplay(t *testing.T) {
	common.CryptoSecret = "test-altcha-secret"
	t.Cleanup(func() { common.CryptoSecret = "" })

	ch, err := CreateAltchaChallenge()
	require.NoError(t, err)
	require.Equal(t, "SHA-256", ch.Algorithm)
	require.NotEmpty(t, ch.Salt)
	require.NotEmpty(t, ch.Signature)

	token := solveAltcha(t, ch)
	require.NoError(t, verifyAltcha(token), "a freshly solved challenge must verify")
	require.Error(t, verifyAltcha(token), "the same challenge must be rejected on replay")
}

func TestAltchaRejectsExpiredAndForgedPayloads(t *testing.T) {
	common.CryptoSecret = "test-altcha-secret"
	t.Cleanup(func() { common.CryptoSecret = "" })

	buildToken := func(salt string, number int64, signature string) string {
		challenge := altchaHash(salt, number)
		if signature == "" {
			signature = altchaSign(challenge)
		}
		payload, err := common.Marshal(map[string]interface{}{
			"algorithm": "SHA-256",
			"challenge": challenge,
			"number":    number,
			"salt":      salt,
			"signature": signature,
		})
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(payload)
	}

	expiredSalt := "abcdef?expires=" + strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	err := verifyAltcha(buildToken(expiredSalt, 42, ""))
	require.Error(t, err, "expired challenges must be rejected")

	// A self-made challenge that was never signed by the server: correct hash,
	// bogus signature. This is the forgery path the HMAC exists to block.
	freshSalt := "abcdef?expires=" + strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	err = verifyAltcha(buildToken(freshSalt, 42, hex.EncodeToString(make([]byte, 32))))
	require.Error(t, err, "unsigned challenges must be rejected")

	// A salt without an embedded expiry must never verify, otherwise old
	// challenges would live forever.
	payload, mErr := common.Marshal(map[string]interface{}{
		"algorithm": "SHA-256",
		"challenge": altchaHash("no-expiry-salt", 7),
		"number":    int64(7),
		"salt":      "no-expiry-salt",
		"signature": altchaSign(altchaHash("no-expiry-salt", 7)),
	})
	require.NoError(t, mErr)
	err = verifyAltcha(base64.StdEncoding.EncodeToString(payload))
	require.Error(t, err)
}

// --- GeeTest ----------------------------------------------------------------

func TestGeetestVerifySendsSignedFormAndAcceptsSuccess(t *testing.T) {
	common.GeetestCaptchaId = "test-captcha-id"
	common.GeetestCaptchaKey = "test-captcha-key"
	t.Cleanup(func() {
		common.GeetestCaptchaId = ""
		common.GeetestCaptchaKey = ""
		geetestValidateURL = "https://gcaptcha4.geetest.com/validate"
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "test-captcha-id", r.URL.Query().Get("captcha_id"))
		assert.Equal(t, "lot-1", r.Form.Get("lot_number"))
		assert.Equal(t, "out-1", r.Form.Get("captcha_output"))
		assert.Equal(t, "pass-1", r.Form.Get("pass_token"))
		assert.Equal(t, "1751400000", r.Form.Get("gen_time"))
		mac := hmac.New(sha256.New, []byte("test-captcha-key"))
		mac.Write([]byte("lot-1"))
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Form.Get("sign_token"),
			"sign_token must be HMAC-SHA256(captcha_key, lot_number)")
		w.Write([]byte(`{"status":"success","result":"success"}`))
	}))
	defer server.Close()
	geetestValidateURL = server.URL

	token := `{"lot_number":"lot-1","captcha_output":"out-1","pass_token":"pass-1","gen_time":"1751400000"}`
	require.NoError(t, verifyGeetest(token))
}

func TestGeetestVerifyFailureModes(t *testing.T) {
	common.GeetestCaptchaId = "test-captcha-id"
	common.GeetestCaptchaKey = "test-captcha-key"
	t.Cleanup(func() {
		common.GeetestCaptchaId = ""
		common.GeetestCaptchaKey = ""
		geetestValidateURL = "https://gcaptcha4.geetest.com/validate"
	})
	validToken := `{"lot_number":"lot-1","captcha_output":"o","pass_token":"p","gen_time":"1"}`

	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","result":"fail","reason":"pass_token expire"}`))
	}))
	defer rejecting.Close()
	geetestValidateURL = rejecting.URL
	require.Error(t, verifyGeetest(validToken), "an upstream reject must fail the check")

	require.Error(t, verifyGeetest("not-json"), "malformed tokens must fail before any upstream call")

	// Official GeeTest disaster-recovery contract: an unreachable validate API
	// fails open so a vendor outage cannot lock users out.
	geetestValidateURL = "http://127.0.0.1:1"
	require.NoError(t, verifyGeetest(validToken))

	common.GeetestCaptchaId = ""
	require.Error(t, verifyGeetest(validToken), "missing configuration must fail closed")
}

// --- Tencent ----------------------------------------------------------------

func TestTencentVerifyBuildsSignedRequestAndAcceptsPass(t *testing.T) {
	common.CaptchaProvider = "tencent"
	common.TencentCaptchaAppId = "190000000"
	common.TencentCaptchaAppSecretKey = "app-secret"
	common.TencentCloudSecretId = "AKIDtest"
	common.TencentCloudSecretKey = "cloud-secret"
	t.Cleanup(func() {
		common.CaptchaProvider = "turnstile"
		common.TencentCaptchaAppId = ""
		common.TencentCaptchaAppSecretKey = ""
		common.TencentCloudSecretId = ""
		common.TencentCloudSecretKey = ""
		tencentCaptchaURL = "https://captcha.intl.tencentcloudapi.com/"
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DescribeCaptchaResult", r.Header.Get("X-TC-Action"))
		assert.Equal(t, "2019-07-22", r.Header.Get("X-TC-Version"))
		assert.NotEmpty(t, r.Header.Get("X-TC-Timestamp"))
		auth := r.Header.Get("Authorization")
		assert.True(t, strings.HasPrefix(auth, "TC3-HMAC-SHA256 Credential=AKIDtest/"), auth)
		assert.Contains(t, auth, "/captcha/tc3_request")
		assert.Contains(t, auth, "SignedHeaders=content-type;host;x-tc-action")

		var body map[string]interface{}
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, float64(9), body["CaptchaType"])
		assert.Equal(t, float64(190000000), body["CaptchaAppId"])
		assert.Equal(t, "ticket-1", body["Ticket"])
		assert.Equal(t, "rand-1", body["Randstr"])
		assert.Equal(t, "1.2.3.4", body["UserIp"])
		w.Write([]byte(`{"Response":{"CaptchaCode":1,"CaptchaMsg":"OK","RequestId":"r"}}`))
	}))
	defer server.Close()
	tencentCaptchaURL = server.URL + "/"

	require.NoError(t, verifyTencent(`{"ticket":"ticket-1","randstr":"rand-1"}`, "1.2.3.4"))
}

func TestTencentVerifyFailureModes(t *testing.T) {
	common.TencentCaptchaAppId = "190000000"
	common.TencentCaptchaAppSecretKey = "app-secret"
	common.TencentCloudSecretId = "AKIDtest"
	common.TencentCloudSecretKey = "cloud-secret"
	t.Cleanup(func() {
		common.TencentCaptchaAppId = ""
		common.TencentCaptchaAppSecretKey = ""
		common.TencentCloudSecretId = ""
		common.TencentCloudSecretKey = ""
		tencentCaptchaURL = "https://captcha.intl.tencentcloudapi.com/"
	})
	validToken := `{"ticket":"t","randstr":"r"}`

	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Response":{"CaptchaCode":7,"CaptchaMsg":"captcha no match"}}`))
	}))
	defer rejecting.Close()
	tencentCaptchaURL = rejecting.URL + "/"
	require.Error(t, verifyTencent(validToken, "1.2.3.4"), "a rejected ticket must fail")

	apiError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"bad sign"}}}`))
	}))
	defer apiError.Close()
	tencentCaptchaURL = apiError.URL + "/"
	require.Error(t, verifyTencent(validToken, "1.2.3.4"), "API auth errors must fail closed")

	// Vendor outage fails open, mirroring the GeeTest channel's posture.
	tencentCaptchaURL = "http://127.0.0.1:1/"
	require.NoError(t, verifyTencent(validToken, "1.2.3.4"))

	require.Error(t, verifyTencent("not-json", "1.2.3.4"))

	common.TencentCaptchaAppId = "not-a-number"
	require.Error(t, verifyTencent(validToken, "1.2.3.4"), "non-numeric CaptchaAppId must fail closed")
}

// --- Turnstile ---------------------------------------------------------------

func TestTurnstileVerifyKeepsFailClosedContract(t *testing.T) {
	common.TurnstileSecretKey = "turnstile-secret"
	t.Cleanup(func() {
		common.TurnstileSecretKey = ""
		turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	})

	accepting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "turnstile-secret", r.Form.Get("secret"))
		assert.Equal(t, "tok", r.Form.Get("response"))
		assert.Equal(t, "1.2.3.4", r.Form.Get("remoteip"))
		w.Write([]byte(`{"success":true}`))
	}))
	defer accepting.Close()
	turnstileVerifyURL = accepting.URL
	require.NoError(t, verifyTurnstile("tok", "1.2.3.4"))

	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false}`))
	}))
	defer rejecting.Close()
	turnstileVerifyURL = rejecting.URL
	require.Error(t, verifyTurnstile("tok", "1.2.3.4"))

	// Unlike GeeTest/Tencent, Turnstile keeps its historical fail-closed
	// behavior on network errors.
	turnstileVerifyURL = "http://127.0.0.1:1"
	require.Error(t, verifyTurnstile("tok", "1.2.3.4"))
}
