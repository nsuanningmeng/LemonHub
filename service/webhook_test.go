package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookPreservesRenderedQuotaContentAndLegacyPlaceholders(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousClient, previousWorker := httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		httpClient, system_setting.WorkerUrl = previousClient, previousWorker
		*fetch = previousFetch
	})
	system_setting.WorkerUrl = ""
	fetch.EnableSSRFProtection = false

	payloads := make(chan WebhookPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := common.DecodeJson(r.Body, &payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		payloads <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient = server.Client()

	content, values := quotaNotifyContent("en", dto.NotifyTypeWebhook, "Quota at 5%", "$1.00")
	for _, tc := range []struct {
		name    string
		content string
		values  []any
		want    string
	}{
		{"rendered quota", content, values, content},
		{"legacy placeholders", "{{value}}: {{value}}; https://example.com/a%20b", []any{"Quota at 5%", "$1.00"}, "Quota at 5%: $1.00; https://example.com/a%20b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, SendWebhookNotify(server.URL, "", dto.NewNotify(dto.NotifyTypeQuotaExceed, "Low quota", tc.content, tc.values)))
			select {
			case payload := <-payloads:
				assert.Equal(t, tc.want, payload.Content)
				assert.Equal(t, tc.values, payload.Values)
				assert.Equal(t, dto.NotifyTypeQuotaExceed, payload.Type)
			default:
				t.Fatal("no webhook payload received")
			}
		})
	}
}
