package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupEmailSuppressions(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&EmailSuppression{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM email_suppressions")
	})
}

// TestUpsertEmailSuppressionEscalation guards the blocking-scope contract:
// hard_bounce (mailbox invalid — blocks everything) must never be downgraded by
// a later complaint (blocks marketing only), while the reverse upgrade must
// apply. Addresses are stored normalized so mixed-case sends still match.
func TestUpsertEmailSuppressionEscalation(t *testing.T) {
	cleanupEmailSuppressions(t)

	require.NoError(t, UpsertEmailSuppression("User@Example.COM ", SuppressionReasonComplaint, SuppressionSourceCallback, "spam report"))
	assert.False(t, IsEmailHardBounced("user@example.com"), "complaint must not block transactional mail")

	// complaint → hard_bounce upgrades.
	require.NoError(t, UpsertEmailSuppression("user@example.com", SuppressionReasonHardBounce, SuppressionSourceSMTP, "550 user unknown"))
	assert.True(t, IsEmailHardBounced("USER@EXAMPLE.COM"))

	// hard_bounce → complaint must NOT downgrade.
	require.NoError(t, UpsertEmailSuppression("user@example.com", SuppressionReasonComplaint, SuppressionSourceCallback, "later complaint"))
	assert.True(t, IsEmailHardBounced("user@example.com"), "hard_bounce must survive a later complaint")

	// Still a single row (upsert, not duplicate insert).
	var count int64
	require.NoError(t, DB.Model(&EmailSuppression{}).Where("email = ?", "user@example.com").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestGetSuppressedEmailSet guards the bulk-send filter contract: one query
// classifies a whole recipient batch, matching case-insensitively, and both
// suppression reasons block marketing mail.
func TestGetSuppressedEmailSet(t *testing.T) {
	cleanupEmailSuppressions(t)

	require.NoError(t, UpsertEmailSuppression("bounce@example.com", SuppressionReasonHardBounce, SuppressionSourceImport, ""))
	require.NoError(t, UpsertEmailSuppression("complaint@example.com", SuppressionReasonComplaint, SuppressionSourceCallback, ""))

	set, err := GetSuppressedEmailSet([]string{"Bounce@Example.com", "complaint@example.com", "clean@example.com", ""})
	require.NoError(t, err)
	assert.Equal(t, SuppressionReasonHardBounce, set["bounce@example.com"])
	assert.Equal(t, SuppressionReasonComplaint, set["complaint@example.com"])
	_, cleanListed := set["clean@example.com"]
	assert.False(t, cleanListed)
}

// TestUpsertEmailSuppressionRejectsInvalidInput locks the validation surface:
// empty addresses and unknown reasons must be refused, not stored.
func TestUpsertEmailSuppressionRejectsInvalidInput(t *testing.T) {
	cleanupEmailSuppressions(t)

	assert.Error(t, UpsertEmailSuppression("   ", SuppressionReasonHardBounce, SuppressionSourceManual, ""))
	assert.Error(t, UpsertEmailSuppression("a@b.com", "banned", SuppressionSourceManual, ""))
}
