package service

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Provider delivery-event ingestion (Aliyun DirectMail and compatible).
//
// Aliyun pushes asynchronous delivery results (bounces, spam complaints,
// deliveries) through EventBridge HTTP targets or MNS HTTP endpoints. The two
// transports wrap the same DirectMail event (rcpt / err_code / event fields) in
// different envelopes, and field naming varies across doc versions, so parsing
// is deliberately tolerant: unwrap known envelopes, then extract fields by any
// of their known aliases. Anything unrecognized is ignored (never an error) —
// the caller must ack with 2xx regardless, or the provider will retry forever.

// EmailDeliveryAction kinds.
const (
	EmailDeliveryActionBounce    = "bounce"    // permanent failure → suppress for everything
	EmailDeliveryActionComplaint = "complaint" // spam report → suppress for marketing
	EmailDeliveryActionIgnore    = "ignore"    // delivered / transient / unrecognized
)

type EmailDeliveryAction struct {
	Recipient string
	Kind      string
	Detail    string
}

// maxDeliveryEventsPerRequest bounds the work a single callback request can
// enqueue — both the number of parsed event objects AND the total emitted
// actions (each action is a DB upsert). The per-event recipient fan-out must
// honor it too, or one event with thousands of ';'-joined recipients would
// bypass the cap and stall the handler on tens of thousands of writes.
const maxDeliveryEventsPerRequest = 1000

// ParseEmailDeliveryEvents extracts suppression actions from a raw callback
// body. It never returns an error: unparseable input yields no actions.
func ParseEmailDeliveryEvents(body []byte) []EmailDeliveryAction {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	var root any
	if err := common.UnmarshalJsonStr(trimmed, &root); err != nil {
		return nil
	}

	events := collectDeliveryEventObjects(root, 0)
	if len(events) > maxDeliveryEventsPerRequest {
		common.SysError(fmt.Sprintf("email delivery events: payload contains %d events, processing first %d", len(events), maxDeliveryEventsPerRequest))
		events = events[:maxDeliveryEventsPerRequest]
	}

	var actions []EmailDeliveryAction
	for _, ev := range events {
		recipient := firstEventString(ev, "rcpt", "rcpt_to", "rcptTo", "to", "recipient", "email", "address")
		if recipient == "" {
			continue
		}
		errCode := firstEventString(ev, "err_code", "errCode", "error_code", "errorCode", "status", "status_code", "statusCode", "code")
		eventType := strings.ToLower(firstEventString(ev, "event", "event_type", "eventType", "type", "action", "message_type", "messageType"))
		detail := fmt.Sprintf("event=%s err_code=%s msg_id=%s", eventType, errCode, firstEventString(ev, "msg_id", "msgId", "env_id", "envId"))

		kind := EmailDeliveryActionIgnore
		switch {
		// Explicit feedback-loop events: the user reported the mail as spam.
		case strings.Contains(eventType, "complaint") || strings.Contains(eventType, "spam") || strings.Contains(eventType, "report"):
			kind = EmailDeliveryActionComplaint
		// Permanent delivery failure (5xx from the receiving server). This
		// includes recipient-side spam blocks; over-suppressing slightly is the
		// reputation-protective choice, and admins can remove entries.
		case strings.HasPrefix(errCode, "5"):
			kind = EmailDeliveryActionBounce
		case strings.Contains(eventType, "bounce") || strings.Contains(eventType, "invalid"):
			kind = EmailDeliveryActionBounce
		}
		if kind == EmailDeliveryActionIgnore {
			continue
		}
		// A single event may carry multiple recipients ("a@x;b@y").
		for _, r := range strings.FieldsFunc(recipient, func(c rune) bool { return c == ';' || c == ',' }) {
			if len(actions) >= maxDeliveryEventsPerRequest {
				common.SysError(fmt.Sprintf("email delivery events: action cap (%d) reached, ignoring the remainder of the payload", maxDeliveryEventsPerRequest))
				return actions
			}
			r = strings.TrimSpace(r)
			// Only well-formed bare addresses may enter the suppression list —
			// callback payloads are semi-trusted and must not seed junk rows.
			addr, err := mail.ParseAddress(r)
			if err != nil || addr.Address != r {
				continue
			}
			actions = append(actions, EmailDeliveryAction{Recipient: r, Kind: kind, Detail: detail})
		}
	}
	return actions
}

// collectDeliveryEventObjects unwraps envelopes (arrays, EventBridge
// CloudEvents `data`, MNS `Message`) down to plain event objects. depth guards
// against pathological nesting.
func collectDeliveryEventObjects(node any, depth int) []map[string]any {
	if depth > 4 {
		return nil
	}
	switch v := node.(type) {
	case []any:
		var out []map[string]any
		for _, item := range v {
			out = append(out, collectDeliveryEventObjects(item, depth+1)...)
		}
		return out
	case map[string]any:
		// EventBridge / CloudEvents: the event sits under "data" (object or
		// JSON string). MNS HTTP push: under "Message" (JSON string, possibly
		// base64-encoded).
		for _, key := range []string{"data", "Data", "Message", "message"} {
			inner, ok := v[key]
			if !ok {
				continue
			}
			switch payload := inner.(type) {
			case map[string]any, []any:
				return collectDeliveryEventObjects(payload, depth+1)
			case string:
				var parsed any
				if err := common.UnmarshalJsonStr(payload, &parsed); err == nil {
					return collectDeliveryEventObjects(parsed, depth+1)
				}
				if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
					if err := common.UnmarshalJsonStr(string(decoded), &parsed); err == nil {
						return collectDeliveryEventObjects(parsed, depth+1)
					}
				}
				// Not JSON — fall through and treat the outer object itself
				// as the event.
			}
			break
		}
		return []map[string]any{v}
	default:
		return nil
	}
}

// firstEventString returns the first present key's value, stringified.
// JSON numbers arrive as float64; err_code in particular may be numeric.
func firstEventString(ev map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := ev[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return fmt.Sprintf("%.0f", v)
		case []any:
			var parts []string
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ";")
			}
		}
	}
	return ""
}

// ApplyEmailDeliveryActions writes the suppressions learned from a callback
// batch and returns how many were applied.
func ApplyEmailDeliveryActions(actions []EmailDeliveryAction) int {
	applied := 0
	for _, action := range actions {
		reason := model.SuppressionReasonHardBounce
		if action.Kind == EmailDeliveryActionComplaint {
			reason = model.SuppressionReasonComplaint
		}
		if err := model.UpsertEmailSuppression(action.Recipient, reason, model.SuppressionSourceCallback, action.Detail); err != nil {
			common.SysError(fmt.Sprintf("email delivery events: failed to suppress %s: %s", action.Recipient, err.Error()))
			continue
		}
		applied++
	}
	return applied
}
