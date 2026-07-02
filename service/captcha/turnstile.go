package captcha

import (
	"errors"
	"net/url"

	"github.com/QuantumNous/new-api/common"
)

var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

func verifyTurnstile(token string, clientIP string) error {
	res, err := httpClient.PostForm(turnstileVerifyURL, url.Values{
		"secret":   {common.TurnstileSecretKey},
		"response": {token},
		"remoteip": {clientIP},
	})
	if err != nil {
		common.SysLog("turnstile siteverify request failed: " + err.Error())
		return errors.New("人机验证服务暂时不可用，请稍后重试")
	}
	defer res.Body.Close()
	var body struct {
		Success bool `json:"success"`
	}
	if err := common.DecodeJson(res.Body, &body); err != nil {
		common.SysLog("turnstile siteverify response invalid: " + err.Error())
		return errors.New("人机验证服务暂时不可用，请稍后重试")
	}
	if !body.Success {
		return errors.New("人机验证失败，请刷新重试")
	}
	return nil
}
