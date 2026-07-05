package service

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseEmailDeliveryEvents locks the callback-ingestion contract across the
// envelope shapes Aliyun DirectMail actually pushes (bare event, event array,
// EventBridge/CloudEvents `data`, MNS `Message` with raw or base64 JSON) and
// the classification rules (5xx → bounce, complaint/spam event → complaint,
// 250/4xx/unknown → ignored).
func TestParseEmailDeliveryEvents(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []EmailDeliveryAction
	}{
		{
			name: "bare event permanent failure",
			body: `{"rcpt":"dead@example.com","err_code":"550","event":"delivery_failed","msg_id":"m1"}`,
			want: []EmailDeliveryAction{{Recipient: "dead@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "array with mixed results",
			body: `[{"rcpt":"ok@example.com","err_code":"250"},{"rcpt":"dead@example.com","err_code":"554"}]`,
			want: []EmailDeliveryAction{{Recipient: "dead@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "cloudevents envelope with data object",
			body: `{"id":"evt-1","source":"acs.directmail","data":{"rcpt":"gone@example.com","err_code":"511"}}`,
			want: []EmailDeliveryAction{{Recipient: "gone@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "mns envelope with json string message",
			body: `{"Message":"{\"rcpt\":\"nobody@example.com\",\"err_code\":\"550\"}","TopicOwner":"x"}`,
			want: []EmailDeliveryAction{{Recipient: "nobody@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "mns envelope with base64 message",
			body: `{"Message":"` + base64.StdEncoding.EncodeToString([]byte(`{"rcpt":"b64@example.com","err_code":"550"}`)) + `"}`,
			want: []EmailDeliveryAction{{Recipient: "b64@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "spam complaint by event type",
			body: `{"rcpt":"angry@example.com","event":"spam_report","err_code":""}`,
			want: []EmailDeliveryAction{{Recipient: "angry@example.com", Kind: EmailDeliveryActionComplaint}},
		},
		{
			name: "numeric err_code",
			body: `{"rcpt":"num@example.com","err_code":554}`,
			want: []EmailDeliveryAction{{Recipient: "num@example.com", Kind: EmailDeliveryActionBounce}},
		},
		{
			name: "multi-recipient event fans out",
			body: `{"rcpt":"a@example.com;b@example.com","err_code":"550"}`,
			want: []EmailDeliveryAction{
				{Recipient: "a@example.com", Kind: EmailDeliveryActionBounce},
				{Recipient: "b@example.com", Kind: EmailDeliveryActionBounce},
			},
		},
		{
			name: "transient 4xx is ignored (provider retries delivery itself)",
			body: `{"rcpt":"slow@example.com","err_code":"450","event":"delivery_failed"}`,
			want: nil,
		},
		{
			name: "delivered is ignored",
			body: `{"rcpt":"fine@example.com","err_code":"250","event":"delivered"}`,
			want: nil,
		},
		{
			name: "garbage input yields nothing",
			body: `this is not json`,
			want: nil,
		},
		{
			name: "event without recipient is ignored",
			body: `{"err_code":"550","event":"delivery_failed"}`,
			want: nil,
		},
		{
			name: "malformed recipients are filtered",
			body: `{"rcpt":"x@;@y.com;good@example.com","err_code":"550"}`,
			want: []EmailDeliveryAction{{Recipient: "good@example.com", Kind: EmailDeliveryActionBounce}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEmailDeliveryEvents([]byte(tt.body))
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].Recipient, got[i].Recipient)
				assert.Equal(t, tt.want[i].Kind, got[i].Kind)
			}
		})
	}
}

// TestParseEmailDeliveryEventsActionCap guards the DoS bound: the per-request
// action cap applies to the TOTAL emitted actions, so a single event fanning
// out thousands of ';'-joined recipients cannot bypass the event-object cap
// and flood the DB with one request.
func TestParseEmailDeliveryEventsActionCap(t *testing.T) {
	recipients := make([]string, 0, 5000)
	for i := 0; i < 5000; i++ {
		recipients = append(recipients, fmt.Sprintf("u%d@example.com", i))
	}
	body := fmt.Sprintf(`{"rcpt":%q,"err_code":"550"}`, strings.Join(recipients, ";"))

	got := ParseEmailDeliveryEvents([]byte(body))
	require.Len(t, got, maxDeliveryEventsPerRequest)
}
