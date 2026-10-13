package service

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEmailCampaignProgressTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSMTPServer, originalSMTPAccount := common.SMTPServer, common.SMTPAccount
	originalMarketingSMTPServer := common.MarketingSMTPServer
	// Even a regression in filtering must never send an email outside the test.
	common.SMTPServer, common.SMTPAccount, common.MarketingSMTPServer = "", "", ""
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SMTPServer, common.SMTPAccount = originalSMTPServer, originalSMTPAccount
		common.MarketingSMTPServer = originalMarketingSMTPServer
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.EmailCampaign{}, &model.User{}, &model.EmailSuppression{}))
	model.DB = db
	// Initialize the database-specific column quoting used by recipient queries.
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	return db
}

func TestRunEmailCampaignCountsSkippedRecipients(t *testing.T) {
	db := setupEmailCampaignProgressTest(t)
	users := []model.User{
		{Id: 1, Username: "invalid", AffCode: "skip1", Email: "not-an-email"},
		{Id: 2, Username: "opted-out", AffCode: "skip2", Email: "opted-out@example.com", Setting: `{"marketing_email_disabled":true}`},
		{Id: 3, Username: "suppressed", AffCode: "skip3", Email: "Suppressed@Example.com"},
		{Id: 4, Username: "opted-out-and-suppressed", AffCode: "skip4", Email: "both@example.com", Setting: `{"marketing_email_disabled":true}`},
		{Id: 5, Username: "no-email", AffCode: "skip5"},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, model.UpsertEmailSuppression("suppressed@example.com", model.SuppressionReasonHardBounce, model.SuppressionSourceManual, ""))
	require.NoError(t, model.UpsertEmailSuppression("both@example.com", model.SuppressionReasonComplaint, model.SuppressionSourceManual, ""))
	campaign := &model.EmailCampaign{Subject: "skip accounting"}
	require.NoError(t, campaign.Insert())

	// The controller retains the original value for its JSON response while a
	// shallow copy is sent in the background. Its skip counter must stay untouched.
	forSend := *campaign
	runEmailCampaign(&forSend, model.SiteScopeAll)
	require.NotNil(t, campaign.SkipCount)
	assert.Zero(t, *campaign.SkipCount)

	got, err := model.GetEmailCampaignById(campaign.Id)
	require.NoError(t, err)
	assert.Equal(t, model.EmailCampaignStatusCompleted, got.Status)
	assert.Equal(t, 4, got.TotalCount)
	require.NotNil(t, got.SkipCount)
	assert.Equal(t, 4, *got.SkipCount)
	assert.Zero(t, got.SentCount)
	assert.Zero(t, got.FailCount)
	assert.Equal(t, 4, got.LastUserId)
	assert.Positive(t, got.FinishedAt)
}

func TestRunEmailCampaignResumesRecordedAndHistoricalProgress(t *testing.T) {
	cases := []struct {
		name       string
		recorded   bool
		sent       int
		wantTotal  int
		wantStatus string
	}{
		{"recorded", true, 7, 11, model.EmailCampaignStatusCompleted},
		{"historical", false, 7, 20, model.EmailCampaignStatusCompleted},
		{"all sends failed", true, 0, 4, model.EmailCampaignStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupEmailCampaignProgressTest(t)
			require.NoError(t, db.Create(&[]model.User{
				{Id: 10, Username: "already-skipped", AffCode: "skip10", Email: "invalid"},
				{Id: 11, Username: "remaining", AffCode: "skip11", Email: "also-invalid"},
			}).Error)
			campaign := &model.EmailCampaign{
				Status:     model.EmailCampaignStatusSending,
				TotalCount: 20, SentCount: tc.sent, FailCount: 2, LastUserId: 10,
			}
			if tc.recorded {
				skipped := 1
				campaign.SkipCount = &skipped
			}
			// Bypass Insert for legacy rows, which predate skip accounting.
			require.NoError(t, db.Create(campaign).Error)
			resumed, err := model.GetEmailCampaignById(campaign.Id)
			require.NoError(t, err)
			runEmailCampaign(resumed, model.SiteScopeAll)

			got, err := model.GetEmailCampaignById(campaign.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, got.Status)
			assert.Equal(t, tc.wantTotal, got.TotalCount)
			assert.Equal(t, tc.sent, got.SentCount)
			assert.Equal(t, 2, got.FailCount)
			assert.Equal(t, 11, got.LastUserId)
			if tc.recorded {
				require.NotNil(t, got.SkipCount)
				assert.Equal(t, 2, *got.SkipCount)
			} else {
				assert.Nil(t, got.SkipCount)
			}
		})
	}
}

func TestRunEmailCampaignParkDoesNotCompleteProgress(t *testing.T) {
	db := setupEmailCampaignProgressTest(t)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "remaining", Email: "invalid"}).Error)
	campaign := &model.EmailCampaign{
		TotalCount: 10, SentCount: 2, LastUserId: 1,
	}
	require.NoError(t, campaign.Insert())
	*campaign.SkipCount = 1
	require.NoError(t, campaign.UpdateProgress())
	// A suppression query failure must park the campaign before advancing its
	// cursor or making the still-unprocessed audience appear complete.
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fail_suppression", func(tx *gorm.DB) {
		if tx.Statement.Table == "email_suppressions" {
			tx.AddError(errors.New("suppression lookup unavailable"))
		}
	}))
	runEmailCampaign(campaign, model.SiteScopeAll)

	got, err := model.GetEmailCampaignById(campaign.Id)
	require.NoError(t, err)
	assert.Equal(t, model.EmailCampaignStatusSending, got.Status)
	assert.Equal(t, 10, got.TotalCount)
	require.NotNil(t, got.SkipCount)
	assert.Equal(t, 1, *got.SkipCount)
	assert.Equal(t, 2, got.SentCount)
	assert.Equal(t, 1, got.LastUserId)
	assert.Zero(t, got.FinishedAt)
}

func TestExpiredEmailCampaignDoesNotCompleteProgress(t *testing.T) {
	db := setupEmailCampaignProgressTest(t)
	campaign := &model.EmailCampaign{
		Status:     model.EmailCampaignStatusSending,
		TotalCount: 10, SentCount: 2, LastUserId: 3,
	}
	require.NoError(t, campaign.Insert())
	*campaign.SkipCount = 1
	require.NoError(t, campaign.UpdateProgress())
	require.NoError(t, db.Model(campaign).Update("updated_at", common.GetTimestamp()-int64(emailCampaignResumeMaxAge/time.Second)-1).Error)

	recoverInterruptedEmailCampaigns()

	got, err := model.GetEmailCampaignById(campaign.Id)
	require.NoError(t, err)
	assert.Equal(t, model.EmailCampaignStatusFailed, got.Status)
	assert.Equal(t, 10, got.TotalCount)
	require.NotNil(t, got.SkipCount)
	assert.Equal(t, 1, *got.SkipCount)
	assert.Equal(t, 2, got.SentCount)
	assert.Equal(t, 3, got.LastUserId)
	assert.Positive(t, got.FinishedAt)
}
