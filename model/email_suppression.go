package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Suppression reason: why an address must not be mailed. The reason decides the
// blocking scope — a hard bounce blocks everything (the mailbox does not exist),
// while a spam complaint blocks marketing only (the user rejected bulk mail, not
// verification codes).
const (
	SuppressionReasonHardBounce = "hard_bounce"
	SuppressionReasonComplaint  = "complaint"
)

// Suppression source: which mechanism added the row.
const (
	SuppressionSourceSMTP     = "smtp"     // synchronous 5xx RCPT rejection during send
	SuppressionSourceCallback = "callback" // async delivery event pushed by the provider
	SuppressionSourceImport   = "import"   // admin-imported invalid-address list
	SuppressionSourceManual   = "manual"   // admin added a single address by hand
)

// maxSuppressionDetailLen bounds the stored error/event snippet.
const maxSuppressionDetailLen = 500

// EmailSuppression is one do-not-mail address. Rows are learned automatically
// from SMTP rejections and provider delivery events, imported from the provider
// console (e.g. Aliyun DirectMail's invalid-address export), or added manually.
// Suppressing known-bad addresses keeps the sender's invalid-address and spam
// rates down, which is what the provider's reputation level is scored on.
type EmailSuppression struct {
	Id        int    `json:"id"`
	Email     string `json:"email" gorm:"type:varchar(255);uniqueIndex;not null"`
	Reason    string `json:"reason" gorm:"type:varchar(32);index"`
	Source    string `json:"source" gorm:"type:varchar(32)"`
	Detail    string `json:"detail" gorm:"type:varchar(512)"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

func normalizeSuppressionEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// UpsertEmailSuppression records (or refreshes) a suppression. A hard_bounce
// reason is never downgraded to complaint: the mailbox not existing is strictly
// stronger information than the user disliking bulk mail.
func UpsertEmailSuppression(email string, reason string, source string, detail string) error {
	email = normalizeSuppressionEmail(email)
	if email == "" {
		return errors.New("empty email")
	}
	if reason != SuppressionReasonHardBounce && reason != SuppressionReasonComplaint {
		return errors.New("invalid suppression reason")
	}
	if runes := []rune(detail); len(runes) > maxSuppressionDetailLen {
		detail = string(runes[:maxSuppressionDetailLen])
	}
	now := common.GetTimestamp()

	var existing EmailSuppression
	err := DB.Where("email = ?", email).First(&existing).Error
	if err == nil {
		if existing.Reason == SuppressionReasonHardBounce {
			reason = SuppressionReasonHardBounce
		}
		return DB.Model(&EmailSuppression{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{
			"reason":     reason,
			"source":     source,
			"detail":     detail,
			"updated_at": now,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	createErr := DB.Create(&EmailSuppression{
		Email:     email,
		Reason:    reason,
		Source:    source,
		Detail:    detail,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
	if createErr == nil {
		return nil
	}
	// Lost a race on the unique index: another writer inserted the row between our
	// lookup and Create. Re-read and apply the same escalation-aware update.
	if err := DB.Where("email = ?", email).First(&existing).Error; err != nil {
		return createErr
	}
	if existing.Reason == SuppressionReasonHardBounce {
		reason = SuppressionReasonHardBounce
	}
	return DB.Model(&EmailSuppression{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{
		"reason":     reason,
		"source":     source,
		"detail":     detail,
		"updated_at": now,
	}).Error
}

// GetSuppressedEmailSet returns reason by normalized email for every address in
// emails that has a suppression row. Used by the bulk-send loop to filter one
// recipient batch with a single query.
func GetSuppressedEmailSet(emails []string) (map[string]string, error) {
	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	normalized := make([]string, 0, len(emails))
	for _, e := range emails {
		if n := normalizeSuppressionEmail(e); n != "" {
			normalized = append(normalized, n)
		}
	}
	if len(normalized) == 0 {
		return map[string]string{}, nil
	}
	var rows []EmailSuppression
	if err := DB.Select("email, reason").Where("email IN ?", normalized).Find(&rows).Error; err != nil {
		return nil, err
	}
	set := make(map[string]string, len(rows))
	for _, r := range rows {
		set[r.Email] = r.Reason
	}
	return set, nil
}

// IsEmailHardBounced reports whether the address is suppressed as a hard bounce
// (mailbox invalid). Transactional sends (verification codes, password resets,
// notifications) consult this; complaint suppressions do NOT block them. Returns
// true only on a confirmed suppression row — lookup errors fail open (the send
// itself will surface any real problem) and are logged.
func IsEmailHardBounced(email string) bool {
	email = normalizeSuppressionEmail(email)
	if email == "" {
		return false
	}
	var row EmailSuppression
	err := DB.Select("id, reason").Where("email = ?", email).First(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError("email suppression lookup failed: " + err.Error())
		}
		return false
	}
	return row.Reason == SuppressionReasonHardBounce
}

// GetEmailSuppressions lists suppressions for the admin console, newest first,
// optionally filtered by an email substring.
func GetEmailSuppressions(keyword string, pageInfo *common.PageInfo) ([]*EmailSuppression, int64, error) {
	var list []*EmailSuppression
	var total int64
	query := DB.Model(&EmailSuppression{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("email LIKE ?", "%"+normalizeSuppressionEmail(keyword)+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func DeleteEmailSuppression(id int) error {
	if id <= 0 {
		return errors.New("invalid suppression id")
	}
	return DB.Delete(&EmailSuppression{}, id).Error
}
