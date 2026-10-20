package common

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resendRoundTripper func(*http.Request) (*http.Response, error)

func (f resendRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendEmailResendRequestAndProviderIsolation(t *testing.T) {
	previousClient, previousOptions, previousName := resendEmailClient, OptionMap, SystemName
	t.Cleanup(func() { resendEmailClient, OptionMap, SystemName = previousClient, previousOptions, previousName })
	OptionMap = map[string]string{"EmailProvider": "resend", "ResendApiKey": "re_test_secret", "ResendFrom": "Support <support@example.com>"}
	SystemName = "Fallback"
	calls := 0
	client := *resendEmailClient
	client.Transport = resendRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		assert.Equal(t, "https://api.resend.com/emails", r.URL.String())
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer re_test_secret", r.Header.Get("Authorization"))
		_, hasDeadline := r.Context().Deadline()
		assert.True(t, hasDeadline)
		var payload struct {
			From    string   `json:"from"`
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			HTML    string   `json:"html"`
		}
		require.NoError(t, DecodeJson(r.Body, &payload))
		assert.Equal(t, `"Support" <support@example.com>`, payload.From)
		assert.Equal(t, []string{"one@example.com", "two@example.com"}, payload.To)
		assert.Equal(t, "subject", payload.Subject)
		assert.Equal(t, "<p>private code</p>", payload.HTML)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"delivered"}`))}, nil
	})
	resendEmailClient = &client
	require.NoError(t, SendEmail("subject", "one@example.com;Two <two@example.com>", "<p>private code</p>"))
	assert.Equal(t, 1, calls)
	OptionMap["EmailProvider"] = "invalid"
	require.Error(t, SendEmail("subject", "one@example.com", "private"))
	assert.Equal(t, 1, calls, "invalid provider cannot silently select SMTP or Resend")
}

func TestResendFailuresDoNotLeakSecretsOrRetry(t *testing.T) {
	previous := resendEmailClient
	t.Cleanup(func() { resendEmailClient = previous })
	var logs bytes.Buffer
	LogWriterMu.Lock()
	previousLogWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	LogWriterMu.Unlock()
	t.Cleanup(func() { LogWriterMu.Lock(); gin.DefaultErrorWriter = previousLogWriter; LogWriterMu.Unlock() })
	const private = "private-code-and-secret-key"
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		transportErr error
		want         string
	}{
		{"transport", 0, "", errors.New(private), "Resend request failed"},
		{"rejected", 403, `{"message":"` + private + `"}`, nil, "Resend email delivery failed (HTTP 403)"},
		{"redirect", 302, "", nil, "Resend email delivery failed (HTTP 302)"},
		{"missing id", 200, `{"message":"` + private + `"}`, nil, "Invalid Resend delivery response"},
		{"malformed", 200, private, nil, "Invalid Resend delivery response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := *previous
			client.Transport = resendRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": {"https://other.example.com"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			resendEmailClient = &client
			err := sendResendEmail(private, "sender@example.com", "title", "receiver@example.com", private)
			require.EqualError(t, err, tc.want)
			assert.NotContains(t, err.Error(), private)
			assert.NotContains(t, logs.String(), private)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestResendConfigurationAndRecipientValidation(t *testing.T) {
	for _, tc := range []struct{ key, from string }{
		{"", "sender@example.com"}, {"secret\ninjected", "sender@example.com"},
		{"secret", "invalid"}, {"secret", "sender..name@example.com"}, {"secret", "sender@example.com\r\nBcc: other@example.com"},
	} {
		require.Error(t, ValidateResendConfiguration(tc.key, tc.from))
	}
	require.NoError(t, ValidateResendConfiguration("secret", "Sender <sender@example.com>"))
	for _, recipient := range []string{"", "invalid", "one@example.com\r\nBcc: two@example.com", strings.Repeat("one@example.com;", 50) + "one@example.com"} {
		require.Error(t, sendResendEmail("secret", "sender@example.com", "title", recipient, "body"))
	}
}

func TestResendSelectionPreservesMarketingSMTPAndUnsubscribe(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	t.Cleanup(server.close)
	withSMTPSettings(t)
	previousOptions := OptionMap
	t.Cleanup(func() { OptionMap = previousOptions })
	OptionMap = map[string]string{"EmailProvider": "resend", "ResendApiKey": "secret", "ResendFrom": "verify@example.com"}
	MarketingSMTPServer, MarketingSMTPPort = server.host, server.port
	MarketingSMTPFrom = "marketing@example.com"
	MarketingSMTPAccount, MarketingSMTPToken = "", ""
	MarketingSMTPSSLEnabled, MarketingSMTPStartTLSEnabled = false, false
	require.NoError(t, SendMarketingEmail("Promo", "receiver@example.com", "<p>news</p>", "List-Unsubscribe: <https://example.com/unsubscribe>", "List-Unsubscribe-Post: List-Unsubscribe=One-Click"))
	select {
	case message := <-server.messages:
		assert.Contains(t, message, "marketing@example.com")
		assert.Contains(t, message, "List-Unsubscribe: <https://example.com/unsubscribe>")
		assert.Contains(t, message, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")
	default:
		t.Fatal("marketing email was not delivered through the configured SMTP transport")
	}
}
