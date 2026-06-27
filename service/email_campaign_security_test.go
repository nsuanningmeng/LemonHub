package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsSafeRecipient guards the bulk-send header-injection / fan-out defense:
// only a single, bare, RFC-5322-parseable address with no CR/LF or ';' is allowed.
func TestIsSafeRecipient(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  bool
	}{
		{"plain address", "a@b.com", true},
		{"crlf header injection", "a@b.com\r\nBcc: c@d.com", false},
		{"semicolon fan-out", "a@b.com;e@f.com", false},
		{"empty", "", false},
		{"display-name form", "Name <a@b.com>", false},
		{"not an email", "not-an-email", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSafeRecipient(tc.email))
		})
	}
}
