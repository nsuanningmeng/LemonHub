package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var geetestValidateURL = "https://gcaptcha4.geetest.com/validate"

// geetestToken carries the proof and optional captcha ID returned by the
// GeeTest v4 widget's getValidate(), JSON-encoded by the frontend.
type geetestToken struct {
	CaptchaID     string `json:"captcha_id"`
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

func verifyGeetest(token string) error {
	captchaID := strings.TrimSpace(common.GeetestCaptchaId)
	captchaKey := strings.TrimSpace(common.GeetestCaptchaKey)
	if captchaID == "" || captchaKey == "" {
		return errors.New("管理员未正确配置极验验证码")
	}
	if strings.Contains(captchaKey, "*") {
		return errors.New("极验验证 Key 不完整，请联系管理员从极验控制台复制完整 Key")
	}
	var t geetestToken
	if err := common.UnmarshalJsonStr(token, &t); err != nil ||
		t.LotNumber == "" || t.CaptchaOutput == "" || t.PassToken == "" || t.GenTime == "" {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	// getValidate() includes the widget's ID. A page opened before a settings
	// change can still hold a proof for the previous ID; never use it with the
	// new key or let the client choose which configured ID to verify against.
	if t.CaptchaID != "" && t.CaptchaID != captchaID {
		return errors.New("人机验证配置已更新，请刷新页面后重试")
	}
	mac := hmac.New(sha256.New, []byte(captchaKey))
	mac.Write([]byte(t.LotNumber))
	signToken := hex.EncodeToString(mac.Sum(nil))

	res, err := httpClient.PostForm(
		geetestValidateURL+"?captcha_id="+url.QueryEscape(captchaID),
		url.Values{
			"lot_number":     {t.LotNumber},
			"captcha_output": {t.CaptchaOutput},
			"pass_token":     {t.PassToken},
			"gen_time":       {t.GenTime},
			"sign_token":     {signToken},
		},
	)
	if err != nil {
		// GeeTest's official disaster-recovery guidance is to fail open when
		// the validate API itself is unreachable, so a GeeTest outage cannot
		// lock every real user out of login. The token here is not
		// client-forgeable into this branch: only our own outbound network
		// failure reaches it.
		common.SysLog("geetest validate unreachable, failing open: " + err.Error())
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		common.SysLog("geetest validate returned " + res.Status + ", failing open")
		return nil
	}
	var body struct {
		Status string          `json:"status"`
		Code   json.RawMessage `json:"code"`
		Msg    json.RawMessage `json:"msg"`
		Result string          `json:"result"`
		Reason string          `json:"reason"`
	}
	if err := common.DecodeJson(res.Body, &body); err != nil {
		common.SysLog("geetest validate response invalid, failing open: " + err.Error())
		return nil
	}
	if body.Status == "error" || body.Result != "success" {
		// API errors use code/msg rather than result/reason. Keep both shapes
		// diagnosable without logging the key, signature or verification token.
		// Diagnostic field types must not trigger the decode-error fallback.
		code := common.JsonRawMessageToString(body.Code)
		common.SysLog(fmt.Sprintf("geetest validate rejected: status=%q code=%q msg=%q result=%q reason=%q",
			body.Status, code, common.JsonRawMessageToString(body.Msg), body.Result, body.Reason))
		if code == "-50304" {
			return errors.New("极验验证签名不匹配，请联系管理员检查验证 ID 和 Key")
		}
		return errors.New("人机验证失败，请刷新重试")
	}
	return nil
}
