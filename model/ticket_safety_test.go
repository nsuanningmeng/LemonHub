package model

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func ticketSafetyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tickets.db")), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Ticket{}, &TicketMessage{}, &TicketAttachment{}))
	oldDB := DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, oldLog)
	t.Cleanup(func() {
		DB = oldDB
		common.SetDatabaseTypes(oldMain, oldLog)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestTicketCreateAndReplyRollBackUnavailableAttachments(t *testing.T) {
	db := ticketSafetyDB(t)
	attachment := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/preserve.png"}
	require.NoError(t, db.Create(&attachment).Error)
	ticket := Ticket{UserId: 7, SiteId: 2, Title: "preserve", Status: TicketStatusOpen, Priority: TicketPriorityNormal}
	message := TicketMessage{UserId: 7, Content: "first"}
	require.ErrorIs(t, CreateTicketWithMessage(&ticket, &message, []int{attachment.Id, attachment.Id + 100}), ErrTicketAttachmentUnavailable)
	for _, model := range []interface{}{&Ticket{}, &TicketMessage{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		assert.Zero(t, count, "a partial submission must roll back every new row")
	}
	require.NoError(t, db.First(&attachment, attachment.Id).Error)
	assert.Zero(t, attachment.MessageId)

	ticket.Id, message.Id = 0, 0
	require.NoError(t, CreateTicketWithMessage(&ticket, &message, nil))
	before := ticket
	reply := TicketMessage{UserId: 7, Content: "should roll back"}
	require.ErrorIs(t, ReplyToTicket(&ticket, &reply, []int{attachment.Id, attachment.Id + 100}), ErrTicketAttachmentUnavailable)
	var messages int64
	require.NoError(t, db.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&messages).Error)
	assert.EqualValues(t, 1, messages)
	var after Ticket
	require.NoError(t, db.First(&after, ticket.Id).Error)
	assert.Equal(t, before, after)
	require.NoError(t, db.First(&attachment, attachment.Id).Error)
	assert.Zero(t, attachment.MessageId)
}

func TestTicketCleanupRechecksStaleOrphanCandidate(t *testing.T) {
	db := ticketSafetyDB(t)
	attachment := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/attached.png"}
	require.NoError(t, db.Create(&attachment).Error)
	candidates, err := CollectAttachmentsForCleanup(2, 100, 0)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusOpen, Priority: TicketPriorityNormal}
	message := TicketMessage{UserId: 7, Content: "attach between scan and cleanup"}
	require.NoError(t, CreateTicketWithMessage(&ticket, &message, []int{attachment.Id}))
	removed, err := DeleteEligibleTicketAttachment(candidates[0].Id, 2, 100, 0)
	require.NoError(t, err)
	assert.Nil(t, removed, "stale orphan candidate must not authorize file removal")
	require.NoError(t, db.First(&attachment, attachment.Id).Error)
	assert.Equal(t, message.Id, attachment.MessageId)

	other := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/expired.png"}
	require.NoError(t, db.Create(&other).Error)
	removed, err = DeleteEligibleTicketAttachment(other.Id, 2, 100, 0)
	require.NoError(t, err)
	require.NotNil(t, removed)
	reply := TicketMessage{UserId: 7, Content: "cleanup already won"}
	require.ErrorIs(t, ReplyToTicket(&ticket, &reply, []int{other.Id}), ErrTicketAttachmentUnavailable)
	var messages int64
	require.NoError(t, db.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&messages).Error)
	assert.EqualValues(t, 1, messages)
}

func TestTicketCleanupPreservesReopenedTicketsAndRejectsPurgedReplies(t *testing.T) {
	db := ticketSafetyDB(t)
	ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusClosed, ClosedAt: 1, Priority: TicketPriorityNormal}
	message := TicketMessage{UserId: 7, Content: "original"}
	attachment := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/closed.png"}
	require.NoError(t, db.Create(&attachment).Error)
	require.NoError(t, CreateTicketWithMessage(&ticket, &message, []int{attachment.Id}))
	ids, err := GetClosedTicketIdsBefore(2, 100, 0)
	require.NoError(t, err)
	require.Equal(t, []int{ticket.Id}, ids)
	require.NoError(t, ticket.UpdateDetails(TicketStatusOpen, ""))
	removed, err := DeleteEligibleTicketAttachment(attachment.Id, 2, 0, 100)
	require.NoError(t, err)
	assert.Nil(t, removed)
	files, messages, purged, err := PurgeClosedTicket(ids[0], 2, 100)
	require.NoError(t, err)
	assert.Empty(t, files)
	assert.Zero(t, messages)
	assert.False(t, purged)
	require.NoError(t, db.First(&attachment, attachment.Id).Error)

	require.NoError(t, db.Model(&Ticket{}).Where("id = ?", ticket.Id).Updates(map[string]interface{}{"status": TicketStatusClosed, "closed_at": 1}).Error)
	files, messages, purged, err = PurgeClosedTicket(ticket.Id, 2, 100)
	require.NoError(t, err)
	require.True(t, purged)
	assert.Len(t, files, 1)
	assert.EqualValues(t, 1, messages)
	require.ErrorIs(t, ticket.UpdateDetails(TicketStatusOpen, ""), gorm.ErrRecordNotFound)
	reply := TicketMessage{UserId: 9, IsAdmin: true, Content: "purge already won"}
	require.ErrorIs(t, ReplyToTicket(&ticket, &reply, nil), gorm.ErrRecordNotFound)
	require.NoError(t, db.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&messages).Error)
	assert.Zero(t, messages, "no orphan reply may be committed")
}

func TestTicketPurgeFailureRollsBackMetadataAndDoesNotReleaseFiles(t *testing.T) {
	db := ticketSafetyDB(t)
	ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusClosed, ClosedAt: 1}
	message := TicketMessage{UserId: 7, Content: "preserve"}
	attachment := TicketAttachment{UserId: 7, SiteId: 2, FilePath: "tickets/preserve.png"}
	require.NoError(t, db.Create(&attachment).Error)
	require.NoError(t, CreateTicketWithMessage(&ticket, &message, []int{attachment.Id}))
	forced := errors.New("delete messages failed")
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:ticket_purge_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "ticket_messages" {
			tx.AddError(forced)
		}
	}))
	files, messages, purged, err := PurgeClosedTicket(ticket.Id, 2, 100)
	require.ErrorIs(t, err, forced)
	assert.Nil(t, files, "the caller must not receive permission to remove disk files on failure")
	assert.Zero(t, messages)
	assert.False(t, purged)
	require.NoError(t, db.First(&ticket, ticket.Id).Error)
	require.NoError(t, db.First(&message, message.Id).Error)
	require.NoError(t, db.First(&attachment, attachment.Id).Error)
}

func TestTicketCleanupUnknownCommitNeverReleasesFiles(t *testing.T) {
	for _, mode := range []string{"commit_not_applied", "commit_applied"} {
		t.Run(mode, func(t *testing.T) {
			db := ticketSafetyDB(t)
			attachment := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/preserve-on-unknown-commit.png"}
			require.NoError(t, db.Create(&attachment).Error)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			forced := errors.New("commit acknowledgement unavailable")
			pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: forced}
			db.ConnPool, db.Statement.ConnPool = pool, pool
			removed, err := DeleteEligibleTicketAttachment(attachment.Id, 2, 100, 0)
			require.ErrorIs(t, err, forced)
			assert.Nil(t, removed, "the filesystem deletion path must not run for either uncertain commit outcome")
			var count int64
			require.NoError(t, db.Model(&TicketAttachment{}).Where("id = ?", attachment.Id).Count(&count).Error)
			if mode == "commit_applied" {
				assert.Zero(t, count)
			} else {
				assert.EqualValues(t, 1, count)
			}
		})
	}
}

func TestTicketBindingRequiresOwnerAndUserSite(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		attachmentUser int
		attachmentSite int
		admin          bool
		allowed        bool
	}{
		{name: "other uploader", attachmentUser: 8, attachmentSite: 2},
		{name: "other user site", attachmentUser: 7, attachmentSite: 3},
		{name: "admin own upload moved to authorized ticket site", attachmentUser: 7, attachmentSite: 0, admin: true, allowed: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			db := ticketSafetyDB(t)
			attachment := TicketAttachment{UserId: fixture.attachmentUser, SiteId: fixture.attachmentSite}
			require.NoError(t, db.Create(&attachment).Error)
			ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusOpen}
			message := TicketMessage{UserId: 7, IsAdmin: fixture.admin, Content: "attach"}
			err := CreateTicketWithMessage(&ticket, &message, []int{attachment.Id})
			if fixture.allowed {
				require.NoError(t, err)
				require.NoError(t, db.First(&attachment, attachment.Id).Error)
				assert.Equal(t, 2, attachment.SiteId)
				assert.Equal(t, message.Id, attachment.MessageId)
			} else {
				require.ErrorIs(t, err, ErrTicketAttachmentUnavailable)
			}
		})
	}
}
