package model

import (
	"maps"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResendOptionsAtomicActivationAndReload(t *testing.T) {
	db := optionPersistenceDB(t)
	require.NoError(t, InitOptionMap())
	assert.Equal(t, "smtp", common.OptionMap["EmailProvider"])
	require.Error(t, UpdateOption("EmailProvider", "resend"))
	assert.Equal(t, "smtp", common.OptionMap["EmailProvider"])
	requireOptionMissing(t, db, "EmailProvider")
	requireOptionMissing(t, db, "ResendApiKey")
	requireOptionMissing(t, db, "ResendFrom")
	require.NoError(t, UpdateOptionsBulk(map[string]string{"EmailProvider": "resend", "ResendApiKey": "re_test_key", "ResendFrom": "Sender <sender@example.com>"}))
	require.Equal(t, "resend", requireOptionValue(t, db, "EmailProvider"))
	for _, values := range []map[string]string{
		{"ResendApiKey": ""}, {"ResendFrom": "sender..invalid@example.com"}, {"EmailProvider": "unknown"}, {"ResendKeyConfigured": "true"},
	} {
		require.Error(t, UpdateOptionsBulk(values))
	}
	assert.Equal(t, "re_test_key", requireOptionValue(t, db, "ResendApiKey"))
	assert.Equal(t, "Sender <sender@example.com>", requireOptionValue(t, db, "ResendFrom"))
	// A process restart must validate the persisted bundle rather than an empty
	// or unrelated runtime map, and must retain all configured credentials.
	common.OptionMap = map[string]string{}
	require.NoError(t, InitOptionMap())
	assert.Equal(t, "resend", common.OptionMap["EmailProvider"])
	assert.Equal(t, "re_test_key", common.OptionMap["ResendApiKey"])
	assert.Equal(t, "Sender <sender@example.com>", common.OptionMap["ResendFrom"])
	require.NoError(t, loadOptionsFromDatabase())
	// Switching to SMTP and clearing Resend together is a valid atomic edit.
	require.NoError(t, UpdateOptionsBulk(map[string]string{"EmailProvider": "smtp", "ResendApiKey": "", "ResendFrom": ""}))
	assert.Equal(t, "smtp", common.OptionMap["EmailProvider"])
	assert.Empty(t, common.OptionMap["ResendApiKey"])
}

func TestResendOptionsValidateDatabaseInsteadOfStaleNodeCache(t *testing.T) {
	for _, active := range []bool{false, true} {
		name := "activate using credentials saved by another node"
		if active {
			name = "reject clearing credentials after another node activated"
		}
		t.Run(name, func(t *testing.T) {
			db := optionPersistenceDB(t)
			require.NoError(t, InitOptionMap())
			staleNode := maps.Clone(common.OptionMap)

			// The second node writes to the shared database while the first
			// retains its pre-SyncOptions snapshot.
			otherNode := map[string]string{
				"ResendApiKey": "re_other_node",
				"ResendFrom":   "other@example.com",
			}
			if active {
				otherNode["EmailProvider"] = "resend"
			}
			require.NoError(t, UpdateOptionsBulk(otherNode))
			common.OptionMap = staleNode

			if active {
				require.Error(t, UpdateOption("ResendApiKey", ""))
			} else {
				require.NoError(t, UpdateOption("EmailProvider", "resend"))
				assert.Equal(t, "re_other_node", common.OptionMap["ResendApiKey"])
				assert.Equal(t, "other@example.com", common.OptionMap["ResendFrom"])
			}
			assert.Equal(t, "resend", requireOptionValue(t, db, "EmailProvider"))
			assert.Equal(t, "re_other_node", requireOptionValue(t, db, "ResendApiKey"))
			assert.Equal(t, "other@example.com", requireOptionValue(t, db, "ResendFrom"))
			require.NoError(t, InitOptionMap(), "the shared snapshot must remain restartable")
		})
	}
}

func TestResendInvalidStoredSnapshotDoesNotReplaceLiveState(t *testing.T) {
	db := optionPersistenceDB(t)
	require.NoError(t, UpdateOptionsBulk(map[string]string{"EmailProvider": "resend", "ResendApiKey": "re_existing", "ResendFrom": "sender@example.com"}))
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "ResendFrom").Update("value", "invalid").Error)
	require.Error(t, loadOptionsFromDatabase())
	assert.Equal(t, "sender@example.com", common.OptionMap["ResendFrom"])
	require.Error(t, InitOptionMap())
	assert.Equal(t, "re_existing", common.OptionMap["ResendApiKey"])
	assert.Equal(t, "sender@example.com", common.OptionMap["ResendFrom"])
}
