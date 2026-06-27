package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsValidTicketPriority locks the accepted ticket priority whitelist, which the
// create/update handlers rely on to reject arbitrary client-supplied priorities.
func TestIsValidTicketPriority(t *testing.T) {
	cases := []struct {
		priority string
		want     bool
	}{
		{"low", true},
		{"normal", true},
		{"high", true},
		{"urgent", true},
		{"", false},
		{"bogus", false},
		{"Low", false},
		{"URGENT", false},
	}
	for _, tc := range cases {
		t.Run(tc.priority, func(t *testing.T) {
			assert.Equal(t, tc.want, IsValidTicketPriority(tc.priority))
		})
	}
}
