package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTicketCleanupKeepsFilesUntilMetadataCommit(t *testing.T) {
	for _, fail := range []bool{true, false} {
		name := "successful cleanup"
		if fail {
			name = "metadata failure"
		}
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tickets.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketAttachment{}))
			oldDB := model.DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB = db
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, oldLog)
			t.Cleanup(func() {
				model.DB = oldDB
				common.SetDatabaseTypes(oldMain, oldLog)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			})
			base := t.TempDir()
			t.Setenv("UPLOAD_PATH", base)
			path := filepath.Join(base, "keep.png")
			require.NoError(t, os.WriteFile(path, []byte("original attachment bytes"), 0600))
			attachment := model.TicketAttachment{UserId: 7, SiteId: 2, FilePath: "keep.png"}
			require.NoError(t, db.Create(&attachment).Error)
			ticket := model.Ticket{UserId: 7, SiteId: 2, Status: model.TicketStatusClosed, ClosedAt: 1}
			message := model.TicketMessage{UserId: 7, Content: "preserve original"}
			require.NoError(t, model.CreateTicketWithMessage(&ticket, &message, []int{attachment.Id}))
			if fail {
				require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("test:cleanup_metadata_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "ticket_messages" {
						tx.AddError(errors.New("injected metadata failure"))
					}
				}))
			}
			result, err := CleanupTicketData(2, 0, 0, 100)
			require.NoError(t, err)
			if fail {
				require.Len(t, result.Errors, 1)
				assert.Zero(t, result.DeletedFiles)
				assert.Zero(t, result.DeletedRows)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "original attachment bytes", string(data))
				require.NoError(t, db.First(&attachment, attachment.Id).Error)
				require.NoError(t, db.First(&message, message.Id).Error)
			} else {
				assert.Empty(t, result.Errors)
				assert.Equal(t, 1, result.DeletedFiles)
				assert.Equal(t, 1, result.DeletedRows)
				assert.Equal(t, 1, result.DeletedMessages)
				assert.Equal(t, 1, result.DeletedTickets)
				_, err := os.Stat(path)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}
