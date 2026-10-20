package service

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func NotifyRootUser(t string, subject string, content string) {
	NotifyRootUserWithEmailContent(t, subject, content, "")
}

// NotifyRootUserWithEmailContent is NotifyRootUser with an email-only HTML body.
// content is the plain-text payload delivered to non-email channels; emailContent
// (when non-empty) is the HTML rendering used only on the email channel.
func NotifyRootUserWithEmailContent(t string, subject string, content string, emailContent string) {
	root := model.GetRootUser()
	if root == nil || root.Id == 0 {
		return
	}
	user := root.ToBaseUser()
	notify := dto.NewNotify(t, subject, content, nil)
	notify.EmailContent = emailContent
	if err := NotifyUser(user.Id, user.Email, user.GetSetting(), notify); err != nil {
		common.SysLog(fmt.Sprintf("failed to notify root user: %s", err.Error()))
	}
}

// NotifyChannelWatchers sends global channel notices only to main-site
// administrators. Sub-site owners must not receive global upstream details.
func NotifyChannelWatchers(notifyType string, render func(lang string) (string, string)) {
	notifyAdmins(notifyType, func(role int, setting dto.UserSetting) bool {
		if setting.ChannelNotifyEnabled != nil {
			return *setting.ChannelNotifyEnabled
		}
		return role == common.RoleRootUser
	}, render)
}

// Keep this API for the existing upstream monitor's message renderer.
func NotifyUpstreamModelUpdateWatchers(subject string, content string) {
	sent := notifyAdmins(dto.NotifyTypeChannelUpdate, func(_ int, setting dto.UserSetting) bool {
		return setting.UpstreamModelUpdateNotifyEnabled
	}, func(string) (string, string) { return subject, content })
	common.SysLog(fmt.Sprintf("upstream model update notifications sent: %d", sent))
}

func notifyAdmins(notifyType string, receives func(int, dto.UserSetting) bool, render func(string) (string, string)) int {
	var users []model.User
	if err := model.DB.Select("id", "email", "role", "status", "setting").
		Where("status = ? AND role IN ? AND site_id = ?", common.UserStatusEnabled, []int{common.RoleAdminUser, common.RoleRootUser}, 0).
		Find(&users).Error; err != nil {
		common.SysLog("failed to query administrators to notify")
		return 0
	}
	sent := 0
	for _, user := range users {
		var setting dto.UserSetting
		if user.Setting != "" {
			if err := common.UnmarshalJsonStr(user.Setting, &setting); err != nil {
				common.SysLog(fmt.Sprintf("skipped administrator %d with invalid notification settings", user.Id))
				continue
			}
		}
		if !receives(user.Role, setting) || !authz.Can(user.Id, user.Role, authz.ChannelRead) {
			continue
		}
		subject, content := render(setting.Language)
		notice := dto.NewNotify(notifyType, subject, content, nil)
		// Channel names and upstream failure text are untrusted plain text.
		// Preserve the webhook/Bark/Gotify payload while escaping email HTML.
		notice.EmailContent = strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>")
		if err := NotifyUser(user.Id, user.Email, setting, notice); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify administrator %d: %s", user.Id, err.Error()))
			continue
		}
		sent++
	}
	return sent
}

func NotifyUser(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
	notifyType := userSetting.NotifyType
	if notifyType == "" {
		notifyType = dto.NotifyTypeEmail
	}

	// Check notification limit
	canSend, err := CheckNotificationLimit(userId, data.Type)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check notification limit: %s", err.Error()))
		return err
	}
	if !canSend {
		return fmt.Errorf("notification limit exceeded for user %d with type %s", userId, notifyType)
	}

	switch notifyType {
	case dto.NotifyTypeEmail:
		// 优先使用设置中的通知邮箱，如果为空则使用用户的默认邮箱
		emailToUse := userSetting.NotificationEmail
		if emailToUse == "" {
			emailToUse = userEmail
		}
		if emailToUse == "" {
			common.SysLog(fmt.Sprintf("user %d has no email, skip sending email", userId))
			return nil
		}
		return sendEmailNotify(emailToUse, data)
	case dto.NotifyTypeWebhook:
		webhookURLStr := userSetting.WebhookUrl
		if webhookURLStr == "" {
			common.SysLog(fmt.Sprintf("user %d has no webhook url, skip sending webhook", userId))
			return nil
		}

		// 获取 webhook secret
		webhookSecret := userSetting.WebhookSecret
		return SendWebhookNotify(webhookURLStr, webhookSecret, data)
	case dto.NotifyTypeBark:
		barkURL := userSetting.BarkUrl
		if barkURL == "" {
			common.SysLog(fmt.Sprintf("user %d has no bark url, skip sending bark", userId))
			return nil
		}
		return sendBarkNotify(barkURL, data)
	case dto.NotifyTypeGotify:
		gotifyUrl := userSetting.GotifyUrl
		gotifyToken := userSetting.GotifyToken
		if gotifyUrl == "" || gotifyToken == "" {
			common.SysLog(fmt.Sprintf("user %d has no gotify url or token, skip sending gotify", userId))
			return nil
		}
		return sendGotifyNotify(gotifyUrl, gotifyToken, userSetting.GotifyPriority, data)
	}
	return nil
}

func sendEmailNotify(userEmail string, data dto.Notify) error {
	// Never notify a hard-bounced address: the mailbox is known-invalid, and each
	// retry counts against the SMTP provider's invalid-address metrics.
	if model.IsEmailHardBounced(userEmail) {
		common.SysLog(fmt.Sprintf("skip email notify to %s: address is on the hard-bounce suppression list", userEmail))
		return nil
	}
	// make email content — prefer the email-only HTML rendering when supplied,
	// otherwise fall back to the shared (plain-text) content.
	content := data.Content
	if data.EmailContent != "" {
		content = data.EmailContent
	}
	// 处理占位符
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	err := common.SendEmail(data.Title, userEmail, content)
	if err != nil {
		RecordEmailSendFailure(userEmail, err)
	}
	return err
}

func sendBarkNotify(barkURL string, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	// 替换模板变量
	finalURL := strings.ReplaceAll(barkURL, "{{title}}", url.QueryEscape(data.Title))
	finalURL = strings.ReplaceAll(finalURL, "{{content}}", url.QueryEscape(content))

	// 发送GET请求到Bark
	var req *http.Request
	var resp *http.Response
	var err error

	if system_setting.EnableWorker() {
		// 使用worker发送请求
		workerReq := &WorkerRequest{
			URL:    finalURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodGet,
			Headers: map[string]string{
				"User-Agent": "OneAPI-Bark-Notify/1.0",
			},
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send bark request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bark request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Bark URL（非Worker模式）
		if err := ValidateSSRFProtectedFetchURL(finalURL); err != nil {
			return fmt.Errorf("request reject: %w", &remoteFetchError{cause: err})
		}

		// 直接发送请求
		req, err = http.NewRequest(http.MethodGet, finalURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create bark request: %w", &remoteFetchError{cause: err})
		}

		// 设置User-Agent
		req.Header.Set("User-Agent", "OneAPI-Bark-Notify/1.0")

		// 发送请求
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send bark request: %w", &remoteFetchError{cause: err})
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bark request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}

func sendGotifyNotify(gotifyUrl string, gotifyToken string, priority int, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	// 构建完整的 Gotify API URL
	// 确保 URL 以 /message 结尾
	finalURL := strings.TrimSuffix(gotifyUrl, "/") + "/message?token=" + url.QueryEscape(gotifyToken)

	// Gotify优先级范围0-10，如果超出范围则使用默认值5
	if priority < 0 || priority > 10 {
		priority = 5
	}

	// 构建 JSON payload
	type GotifyMessage struct {
		Title    string `json:"title"`
		Message  string `json:"message"`
		Priority int    `json:"priority"`
	}

	payload := GotifyMessage{
		Title:    data.Title,
		Message:  content,
		Priority: priority,
	}

	// 序列化为 JSON
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal gotify payload: %v", err)
	}

	var req *http.Request
	var resp *http.Response

	if system_setting.EnableWorker() {
		// 使用worker发送请求
		workerReq := &WorkerRequest{
			URL:    finalURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
				"User-Agent":   "OneAPI-Gotify-Notify/1.0",
			},
			Body: payloadBytes,
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send gotify request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("gotify request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Gotify URL（非Worker模式）
		if err := ValidateSSRFProtectedFetchURL(finalURL); err != nil {
			return fmt.Errorf("request reject: %w", &remoteFetchError{cause: err})
		}

		// 直接发送请求
		req, err = http.NewRequest(http.MethodPost, finalURL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create gotify request: %w", &remoteFetchError{cause: err})
		}

		// 设置请求头
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "NewAPI-Gotify-Notify/1.0")

		// 发送请求
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send gotify request: %w", &remoteFetchError{cause: err})
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("gotify request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}
