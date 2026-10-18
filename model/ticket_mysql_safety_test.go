package model

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This test requires an empty, explicitly disposable schema and coordinates
// real transactions with row-lock barriers rather than sleeps or retries.
func TestMySQLTicketCleanupSerializesWithBindingAndReopen(t *testing.T) {
	rawDSN := strings.TrimSpace(os.Getenv("MYSQL_TICKET_SAFETY_DSN"))
	if rawDSN == "" {
		t.Skip("set MYSQL_TICKET_SAFETY_DSN to a fresh lemonhub_ticket_safety_test_* database")
	}
	config, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^lemonhub_ticket_safety_test_[a-z0-9_]+$`), config.DBName)
	config.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), newGormConfig(false))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	sqlDB.SetMaxOpenConns(4)
	var selected string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selected).Error)
	require.Equal(t, config.DBName, selected)
	var tables int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error)
	require.Zero(t, tables, "refusing to modify a nonempty database")
	require.NoError(t, db.AutoMigrate(&Ticket{}, &TicketMessage{}, &TicketAttachment{}))
	oldDB := DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, oldLog)
	t.Cleanup(func() {
		DB = oldDB
		common.SetDatabaseTypes(oldMain, oldLog)
	})

	t.Run("cleanup owns orphan before reply binds", func(t *testing.T) {
		ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusOpen}
		first := TicketMessage{UserId: 7, Content: "first"}
		require.NoError(t, CreateTicketWithMessage(&ticket, &first, nil))
		attachment := TicketAttachment{UserId: 7, SiteId: 2, CreatedAt: 1, FilePath: "tickets/orphan.png"}
		require.NoError(t, db.Create(&attachment).Error)
		entered, release := holdTicketQueryLock(t, db, "ticket_attachments")
		cleanupDone := make(chan error, 1)
		go func() {
			removed, err := DeleteEligibleTicketAttachment(attachment.Id, 2, 100, 0)
			if err == nil && removed == nil {
				err = errors.New("eligible attachment was not removed")
			}
			cleanupDone <- err
		}()
		waitTicketLock(t, entered)
		replyDone := make(chan error, 1)
		go func() {
			reply := TicketMessage{UserId: 7, Content: "must not partially submit"}
			replyDone <- ReplyToTicket(&ticket, &reply, []int{attachment.Id})
		}()
		release()
		require.NoError(t, <-cleanupDone)
		require.ErrorIs(t, <-replyDone, ErrTicketAttachmentUnavailable)
		var count int64
		require.NoError(t, db.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
		assert.EqualValues(t, 1, count)
	})

	t.Run("reopen owns ticket before purge", func(t *testing.T) {
		ticket := Ticket{UserId: 7, SiteId: 2, Status: TicketStatusClosed, ClosedAt: 1}
		first := TicketMessage{UserId: 7, Content: "preserve on reopen"}
		require.NoError(t, CreateTicketWithMessage(&ticket, &first, nil))
		entered, release := holdTicketQueryLock(t, db, "tickets")
		reopenDone := make(chan error, 1)
		go func() { reopenDone <- ticket.UpdateDetails(TicketStatusOpen, "") }()
		waitTicketLock(t, entered)
		purgeDone := make(chan error, 1)
		ticketID := ticket.Id
		go func() {
			_, _, deleted, err := PurgeClosedTicket(ticketID, 2, 100)
			if err == nil && deleted {
				err = errors.New("purge removed a concurrently reopened ticket")
			}
			purgeDone <- err
		}()
		release()
		require.NoError(t, <-reopenDone)
		require.NoError(t, <-purgeDone)
		var current Ticket
		require.NoError(t, db.First(&current, ticketID).Error)
		assert.Equal(t, TicketStatusOpen, current.Status)
		require.NoError(t, db.First(&first, first.Id).Error)
	})
}

func holdTicketQueryLock(t *testing.T, db *gorm.DB, table string) (<-chan struct{}, func()) {
	t.Helper()
	entered, released := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	const callback = "test:ticket_locked_query"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		_, locked := tx.Statement.Clauses["FOR"]
		if tx.Statement.Table != table || !locked || tx.Error != nil {
			return
		}
		enterOnce.Do(func() {
			close(entered)
			select {
			case <-released:
			case <-time.After(10 * time.Second):
				tx.AddError(errors.New("ticket transaction barrier timed out"))
			}
		})
	}))
	t.Cleanup(func() {
		release()
		require.NoError(t, db.Callback().Query().Remove(callback))
	})
	return entered, release
}

func waitTicketLock(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("transaction did not reach its row-lock barrier")
	}
}
