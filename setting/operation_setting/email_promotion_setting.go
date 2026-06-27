package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// EmailPromotionSetting 邮件推广配置
type EmailPromotionSetting struct {
	// 新增时间线公告时，是否同步群发邮件给用户
	AnnouncementEmailEnabled bool `json:"announcement_email_enabled"`
	// 群发速率上限（封/分钟），用于节流，避免触发 SMTP 限流
	RatePerMinute int `json:"rate_per_minute"`
}

var emailPromotionSetting = EmailPromotionSetting{
	AnnouncementEmailEnabled: false,
	RatePerMinute:            60,
}

func init() {
	config.GlobalConfig.Register("email_promotion_setting", &emailPromotionSetting)
}

// GetEmailPromotionSetting 返回邮件推广配置实例
func GetEmailPromotionSetting() *EmailPromotionSetting {
	return &emailPromotionSetting
}

// maxRatePerMinute caps the configurable bulk-send rate so a misconfigured or
// abusive setting cannot turn the gateway into an SMTP flood source.
const maxRatePerMinute = 600

// GetRatePerMinute 返回带兜底并受上限约束的群发速率（封/分钟）。
func (s *EmailPromotionSetting) GetRatePerMinute() int {
	if s.RatePerMinute <= 0 {
		return 60
	}
	if s.RatePerMinute > maxRatePerMinute {
		return maxRatePerMinute
	}
	return s.RatePerMinute
}
