package ticket_setting

import "github.com/QuantumNous/new-api/setting/config"

// TicketType 单个工单类型定义（管理员可自定义）
type TicketType struct {
	Key            string `json:"key"`             // 唯一标识（用于存储）
	Name           string `json:"name"`            // 展示名称
	PromptTemplate string `json:"prompt_template"` // 自定义提示格式（创建工单时引导用户的模板）
	Enabled        bool   `json:"enabled"`         // 是否启用
}

// TicketSetting 工单系统配置
type TicketSetting struct {
	Enabled                   bool         `json:"enabled"`                      // 是否启用工单系统
	Types                     []TicketType `json:"types"`                        // 自定义工单类型列表
	AdminNotifyEnabled        bool         `json:"admin_notify_enabled"`         // 新工单/新回复是否通知管理员
	AttachmentMaxSizeMB       int          `json:"attachment_max_size_mb"`       // 单个附件大小上限（MB）
	MaxAttachmentsPerMessage  int          `json:"max_attachments_per_message"`  // 单条消息最大附件数
	AllowedMimeTypes          []string     `json:"allowed_mime_types"`           // 允许的图片 MIME 类型白名单
	AttachmentRetentionDays   int          `json:"attachment_retention_days"`    // 已关闭工单附件自动清理天数（0=不自动清理）
	ClosedTicketRetentionDays int          `json:"closed_ticket_retention_days"` // 已关闭工单（含消息）自动清理天数（0=不自动清理）
}

var defaultTicketSetting = TicketSetting{
	Enabled: true,
	Types: []TicketType{
		{Key: "general", Name: "General Question", PromptTemplate: "", Enabled: true},
		{Key: "billing", Name: "Billing & Quota", PromptTemplate: "", Enabled: true},
		{Key: "bug", Name: "Bug Report", PromptTemplate: "", Enabled: true},
	},
	AdminNotifyEnabled:       true,
	AttachmentMaxSizeMB:      5,
	MaxAttachmentsPerMessage: 6,
	AllowedMimeTypes:         []string{"image/png", "image/jpeg", "image/gif", "image/webp"},
	AttachmentRetentionDays:  0,
}

var ticketSetting = defaultTicketSetting

func init() {
	config.GlobalConfig.Register("ticket_setting", &ticketSetting)
}

// GetTicketSetting 返回工单配置实例
func GetTicketSetting() *TicketSetting {
	return &ticketSetting
}

// IsValidType 校验给定 type key 是否为已启用的工单类型
func (s *TicketSetting) IsValidType(key string) bool {
	for _, t := range s.Types {
		if t.Key == key && t.Enabled {
			return true
		}
	}
	return false
}

// IsAllowedMime 校验 MIME 是否在白名单内
func (s *TicketSetting) IsAllowedMime(mime string) bool {
	for _, m := range s.AllowedMimeTypes {
		if m == mime {
			return true
		}
	}
	return false
}

// MaxAttachmentBytes 返回单附件字节上限（带兜底）
func (s *TicketSetting) MaxAttachmentBytes() int64 {
	size := s.AttachmentMaxSizeMB
	if size <= 0 {
		size = 5
	}
	return int64(size) * 1024 * 1024
}
