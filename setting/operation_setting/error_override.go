package operation_setting

import "strings"

// 全局「统一错误信息」兜底：开启后，「无可用渠道/获取渠道失败」这类路由错误
// 以及命中泄密关键词的渠道来源错误，其用户可见文本统一替换为固定文案。
// 其余上游报错（内容审核、上下文超长等对用户有用的信息）原样透传。
// 渠道编辑里的单渠道配置提供该渠道的替换文案（优先于全局文案），
// 屏蔽范围分类与全局一致。
var (
	ErrorOverrideGlobalEnabled = false
	ErrorOverrideGlobalMessage = ""
)

// ErrorOverrideKeywords：命中即视为「泄露渠道/号池信息」的报错关键词
// （不区分大小写，子串匹配）。上游报错提到账号/密钥/额度/组织等，必然
// 指的是渠道自身的账号体系（上游并不认识终端用户），可放心替换。
var ErrorOverrideKeywords = []string{
	"no available",
	"no_available",
	"oauth",
	"account",
	"api key",
	"apikey",
	"api-key",
	"credential",
	"credit",
	"billing",
	"quota",
	"insufficient",
	"organization",
	"security token",
	"access token",
	"unauthorized",
	"permission denied",
	"suspended",
	"disabled",
	"pool",
	"号池",
	"账号",
	"密钥",
	"额度",
	"渠道",
	"分组",
}

func ErrorOverrideKeywordsToString() string {
	return strings.Join(ErrorOverrideKeywords, "\n")
}

func ErrorOverrideKeywordsFromString(s string) {
	ErrorOverrideKeywords = []string{}
	for _, k := range strings.Split(s, "\n") {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			ErrorOverrideKeywords = append(ErrorOverrideKeywords, k)
		}
	}
}

// 与 dto.DefaultErrorOverrideMessage 保持一致（logger→operation_setting→dto 会成环，无法直接引用）
const defaultGlobalErrorOverrideMessage = "上游服务暂时不可用，请稍后重试"

// GlobalErrorOverrideText returns the site-wide fixed error text shown to
// users in place of channel/routing error messages, and whether it is enabled.
func GlobalErrorOverrideText() (string, bool) {
	if !ErrorOverrideGlobalEnabled {
		return "", false
	}
	msg := strings.TrimSpace(ErrorOverrideGlobalMessage)
	if msg == "" {
		msg = defaultGlobalErrorOverrideMessage
	}
	return msg, true
}
