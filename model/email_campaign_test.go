package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupEmailCampaigns(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM email_campaigns")
	})
}

// TestClaimStaleEmailCampaign guards the crash-recovery contract: an orphaned
// (stale pending/sending) campaign is claimable exactly once, while live and
// terminal campaigns are never claimable. This is what prevents two nodes from
// resuming the same interrupted campaign and double-mailing its audience.
func TestClaimStaleEmailCampaign(t *testing.T) {
	cleanupEmailCampaigns(t)
	now := common.GetTimestamp()

	stale := &EmailCampaign{Subject: "s", Content: "c", Status: EmailCampaignStatusSending}
	require.NoError(t, stale.Insert())
	// Simulate a campaign whose sender died long ago.
	require.NoError(t, DB.Model(&EmailCampaign{}).Where("id = ?", stale.Id).
		Update("updated_at", now-3600).Error)

	fresh := &EmailCampaign{Subject: "s", Content: "c", Status: EmailCampaignStatusSending}
	require.NoError(t, fresh.Insert())

	done := &EmailCampaign{Subject: "s", Content: "c", Status: EmailCampaignStatusCompleted}
	require.NoError(t, done.Insert())
	require.NoError(t, DB.Model(&EmailCampaign{}).Where("id = ?", done.Id).
		Update("updated_at", now-3600).Error)

	staleBefore := now - 900

	// Only the stale sending campaign is visible to the sweep.
	list, err := FindStaleActiveCampaigns(staleBefore)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, stale.Id, list[0].Id)

	// First claim wins...
	claimed, err := ClaimStaleEmailCampaign(stale.Id, staleBefore)
	require.NoError(t, err)
	assert.True(t, claimed)

	// ...and refreshes updated_at, so a concurrent sweep on another node loses.
	claimed, err = ClaimStaleEmailCampaign(stale.Id, staleBefore)
	require.NoError(t, err)
	assert.False(t, claimed)

	// A live campaign (recent updated_at) is never claimable.
	claimed, err = ClaimStaleEmailCampaign(fresh.Id, staleBefore)
	require.NoError(t, err)
	assert.False(t, claimed)

	// A terminal campaign is never claimable, however stale.
	claimed, err = ClaimStaleEmailCampaign(done.Id, staleBefore)
	require.NoError(t, err)
	assert.False(t, claimed)
}

// TestClaimZombieCampaignWithNullUpdatedAt guards the upgrade path: campaigns
// stuck in "sending" from before the updated_at column existed have NULL there
// after AutoMigrate. The sweep must still see and claim them (they are then closed
// as failed by the age check), otherwise they hold an active-campaign slot forever.
func TestClaimZombieCampaignWithNullUpdatedAt(t *testing.T) {
	cleanupEmailCampaigns(t)

	zombie := &EmailCampaign{Subject: "s", Content: "c", Status: EmailCampaignStatusSending}
	require.NoError(t, zombie.Insert())
	require.NoError(t, DB.Exec("UPDATE email_campaigns SET updated_at = NULL WHERE id = ?", zombie.Id).Error)

	staleBefore := common.GetTimestamp() - 900

	list, err := FindStaleActiveCampaigns(staleBefore)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, zombie.Id, list[0].Id)
	assert.EqualValues(t, 0, list[0].UpdatedAt)

	claimed, err := ClaimStaleEmailCampaign(zombie.Id, staleBefore)
	require.NoError(t, err)
	assert.True(t, claimed)
}

// TestUpdateProgressPersistsResumeCursor guards the resume contract: the keyset
// cursor and counters written by UpdateProgress must round-trip through the DB,
// because a recovered campaign continues exactly from these values.
func TestUpdateProgressPersistsResumeCursor(t *testing.T) {
	cleanupEmailCampaigns(t)

	c := &EmailCampaign{Subject: "s", Content: "c", Status: EmailCampaignStatusSending}
	require.NoError(t, c.Insert())

	c.SentCount = 7
	c.FailCount = 2
	c.LastUserId = 4321
	require.NoError(t, c.UpdateProgress())

	got, err := GetEmailCampaignById(c.Id)
	require.NoError(t, err)
	assert.Equal(t, 7, got.SentCount)
	assert.Equal(t, 2, got.FailCount)
	assert.Equal(t, 4321, got.LastUserId)
	assert.GreaterOrEqual(t, got.UpdatedAt, c.CreatedAt)
}
