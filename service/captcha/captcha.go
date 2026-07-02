// Package captcha verifies human-verification tokens issued by the
// configured bot-protection channel (Cloudflare Turnstile, GeeTest v4,
// self-hosted ALTCHA, or Tencent Cloud Captcha).
//
// The frontend submits whatever the active widget produced as a single
// opaque string (raw token for Turnstile, JSON for GeeTest/Tencent,
// base64 payload for ALTCHA); each verifier knows how to decode its own
// format, so the transport layer stays provider-agnostic.
package captcha

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	ProviderTurnstile = "turnstile"
	ProviderGeetest   = "geetest"
	ProviderAltcha    = "altcha"
	ProviderTencent   = "tencent"
)

// httpClient bounds every upstream verification call so a hung captcha
// vendor cannot stall the login/registration endpoints indefinitely.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Provider returns the currently configured channel, falling back to
// Turnstile for unknown values so stale options can never disable the check.
func Provider() string {
	switch common.CaptchaProvider {
	case ProviderGeetest, ProviderAltcha, ProviderTencent:
		return common.CaptchaProvider
	default:
		return ProviderTurnstile
	}
}

// ProviderConfigured reports whether the active channel has every credential
// it needs to verify tokens, which gates enabling the bot-protection switch.
func ProviderConfigured() bool {
	switch Provider() {
	case ProviderGeetest:
		return common.GeetestCaptchaId != "" && common.GeetestCaptchaKey != ""
	case ProviderAltcha:
		// The ALTCHA secret is generated automatically at startup.
		return true
	case ProviderTencent:
		return common.TencentCaptchaAppId != "" && common.TencentCaptchaAppSecretKey != "" &&
			common.TencentCloudSecretId != "" && common.TencentCloudSecretKey != ""
	default:
		return common.TurnstileSiteKey != "" && common.TurnstileSecretKey != ""
	}
}

// Verify checks a client captcha token against the active provider.
// A nil return means the token is valid; the returned error message is
// safe to show to end users (details are logged server-side).
func Verify(token string, clientIP string) error {
	switch Provider() {
	case ProviderGeetest:
		return verifyGeetest(token)
	case ProviderAltcha:
		return verifyAltcha(token)
	case ProviderTencent:
		return verifyTencent(token, clientIP)
	default:
		return verifyTurnstile(token, clientIP)
	}
}
