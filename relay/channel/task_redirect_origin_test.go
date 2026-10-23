package channel

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRedirectOriginCredentialBoundary(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		want     bool
	}{
		{"https://api.example/tasks", "https://api.example/result", true},
		{"https://API.example/tasks", "https://api.example:443/result", true},
		{"http://api.example:80/tasks", "http://api.example/result", true},
		{"https://api.example/tasks", "http://api.example/result", false},
		{"https://api.example/tasks", "https://api.example:8443/result", false},
		{"https://api.example/tasks", "https://api.example.attacker.test/result", false},
		{"https://api.example/tasks", "https://other.example/result", false},
	} {
		t.Run(tc.from+" to "+tc.to, func(t *testing.T) {
			first, err := url.Parse(tc.from)
			require.NoError(t, err)
			next, err := url.Parse(tc.to)
			require.NoError(t, err)
			assert.Equal(t, tc.want, sameTaskRedirectOrigin(first, next))
		})
	}
}
