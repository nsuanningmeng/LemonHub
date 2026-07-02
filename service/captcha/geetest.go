package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"

	"github.com/QuantumNous/new-api/common"
)

var geetestValidateURL = "https://gcaptcha4.geetest.com/validate"

// geetestToken carries the four values the GeeTest v4 widget returns from
// getValidate(), JSON-encoded by the frontend into a single token string.
type geetestToken struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

func verifyGeetest(token string) error {
	if common.GeetestCaptchaId == "" || common.GeetestCaptchaKey == "" {
		return errors.New("管理员未正确配置极验验证码")
	}
	var t geetestToken
	if err := common.UnmarshalJsonStr(token, &t); err != nil || t.LotNumber == "" {
		return errors.New("人机验证参数无效，请刷新重试")
	}
	mac := hmac.New(sha256.New, []byte(common.GeetestCaptchaKey))
	mac.Write([]byte(t.LotNumber))
	signToken := hex.EncodeToString(mac.Sum(nil))

	res, err := httpClient.PostForm(
		geetestValidateURL+"?captcha_id="+url.QueryEscape(common.GeetestCaptchaId),
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
		Status string `json:"status"`
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := common.DecodeJson(res.Body, &body); err != nil {
		common.SysLog("geetest validate response invalid, failing open: " + err.Error())
		return nil
	}
	if body.Result != "success" {
		common.SysLog("geetest validate rejected: status=" + body.Status + " reason=" + body.Reason)
		return errors.New("人机验证失败，请刷新重试")
	}
	return nil
}
