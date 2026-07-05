package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnsubscribeTokenRoundTrip guards the one-click unsubscribe credential:
// a token identifies exactly the user it was issued for, and any tampering or
// foreign-secret token is rejected. These tokens are embedded in delivered
// mail, so verification must be stable for a fixed secret.
func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	original := UnsubscribeSecret
	t.Cleanup(func() { UnsubscribeSecret = original })
	UnsubscribeSecret = "test-unsubscribe-secret"

	token, err := GenerateUnsubscribeToken(42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userId, err := ParseUnsubscribeToken(token)
	require.NoError(t, err)
	assert.Equal(t, 42, userId)

	// Tampered token is rejected.
	_, err = ParseUnsubscribeToken(token[:len(token)-2] + "xx")
	assert.Error(t, err)

	// Garbage is rejected.
	_, err = ParseUnsubscribeToken("not-a-token")
	assert.Error(t, err)

	// A token minted under a different secret is rejected.
	UnsubscribeSecret = "rotated-secret"
	_, err = ParseUnsubscribeToken(token)
	assert.Error(t, err)

	// Invalid user ids are refused at mint time.
	UnsubscribeSecret = "test-unsubscribe-secret"
	_, err = GenerateUnsubscribeToken(0)
	assert.Error(t, err)

	// With no secret initialized, minting fails closed.
	UnsubscribeSecret = ""
	_, err = GenerateUnsubscribeToken(42)
	assert.Error(t, err)
}
