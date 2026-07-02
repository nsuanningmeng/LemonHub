package captcha

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Tencent Cloud Captcha (international edition). Verification calls the
// DescribeCaptchaResult action with a hand-rolled TC3-HMAC-SHA256 signature,
// which keeps the dependency tree free of the full Tencent Cloud SDK.
var tencentCaptchaURL = "https://captcha.intl.tencentcloudapi.com/"

const (
	tencentCaptchaAction  = "DescribeCaptchaResult"
	tencentCaptchaVersion = "2019-07-22"
	tencentCaptchaService = "captcha"
)

// tencentToken carries the values the TencentCaptcha widget returns in its
// callback, JSON-encoded by the frontend into a single token string.
type tencentToken struct {
	Ticket  string `json:"ticket"`
	Randstr string `json:"randstr"`
}

func verifyTencent(token string, clientIP string) error {
	if common.TencentCaptchaAppId == "" || common.TencentCaptchaAppSecretKey == "" ||
		common.TencentCloudSecretId == "" || common.TencentCloudSecretKey == "" {
		return errors.New("管理员未正确配置腾讯云验证码")
	}
	appId, err := strconv.ParseUint(common.TencentCaptchaAppId, 10, 64)
	if err != nil {
		return errors.New("管理员未正确配置腾讯云验证码")
	}
	var t tencentToken
	if err := common.UnmarshalJsonStr(token, &t); err != nil || t.Ticket == "" || t.Randstr == "" {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	payload, err := common.Marshal(map[string]interface{}{
		"CaptchaType":  9,
		"Ticket":       t.Ticket,
		"Randstr":      t.Randstr,
		"UserIp":       clientIP,
		"CaptchaAppId": appId,
		"AppSecretKey": common.TencentCaptchaAppSecretKey,
	})
	if err != nil {
		return errors.New("人机验证服务暂时不可用，请稍后重试")
	}

	req, err := http.NewRequest(http.MethodPost, tencentCaptchaURL, bytes.NewReader(payload))
	if err != nil {
		return errors.New("人机验证服务暂时不可用，请稍后重试")
	}
	timestamp := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", tencentCaptchaAction)
	req.Header.Set("X-TC-Version", tencentCaptchaVersion)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("Authorization", tc3Authorization(req.URL.Host, payload, timestamp))

	res, err := httpClient.Do(req)
	if err != nil {
		// Match the GeeTest channel's posture (and Tencent's own sample
		// guidance): an outage of the vendor API must not lock every real
		// user out of login. This branch is only reachable through our own
		// outbound network failure, not through client-supplied data.
		common.SysLog("tencent captcha API unreachable, failing open: " + err.Error())
		return nil
	}
	defer res.Body.Close()
	var body struct {
		Response struct {
			CaptchaCode int64  `json:"CaptchaCode"`
			CaptchaMsg  string `json:"CaptchaMsg"`
			Error       *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := common.DecodeJson(res.Body, &body); err != nil {
		common.SysLog("tencent captcha response invalid, failing open: " + err.Error())
		return nil
	}
	if body.Response.Error != nil {
		// API-level errors (bad credentials, signature mismatch) are operator
		// misconfiguration; fail closed and surface it in the logs.
		common.SysLog("tencent captcha API error: " + body.Response.Error.Code + " " + body.Response.Error.Message)
		return errors.New("人机验证服务配置错误，请联系管理员")
	}
	if body.Response.CaptchaCode != 1 {
		common.SysLog("tencent captcha rejected: code=" + strconv.FormatInt(body.Response.CaptchaCode, 10) + " msg=" + body.Response.CaptchaMsg)
		return errors.New("人机验证失败，请刷新重试")
	}
	return nil
}

// tc3Authorization builds the TC3-HMAC-SHA256 Authorization header for a
// Tencent Cloud API 3.0 POST request with a JSON body.
func tc3Authorization(host string, payload []byte, timestamp int64) string {
	canonicalHeaders := "content-type:application/json; charset=utf-8\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + strings.ToLower(tencentCaptchaAction) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		sha256Hex(payload),
	}, "\n")

	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := date + "/" + tencentCaptchaService + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		strconv.FormatInt(timestamp, 10),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+common.TencentCloudSecretKey), date)
	secretService := hmacSHA256(secretDate, tencentCaptchaService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	return "TC3-HMAC-SHA256 Credential=" + common.TencentCloudSecretId + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
