package dto

type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`
	// EmailContent, when non-empty, is an email-only HTML rendering used by the
	// email channel in place of Content. Plain-text channels (webhook/bark/gotify)
	// always use Content, so callers that want rich HTML in email without leaking
	// raw tags to other channels keep Content plain and set EmailContent to the
	// HTML variant. Tagged json:"-" so it never appears in webhook payloads.
	EmailContent string `json:"-"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed   = "quota_exceed"
	NotifyTypeChannelUpdate = "channel_update"
	NotifyTypeChannelTest   = "channel_test"
)

func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
