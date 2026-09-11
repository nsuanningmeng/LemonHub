package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaPreservesQuotaAndSiteScope(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	logs := []Log{
		{SiteId: 0, CreatedAt: now - 10, Type: LogTypeConsume, Quota: 100, PromptTokens: 10, CompletionTokens: 5},
		{SiteId: 0, CreatedAt: now - 3600, Type: LogTypeConsume, Quota: 50, PromptTokens: 30, CompletionTokens: 10},
		{SiteId: 7, CreatedAt: now - 10, Type: LogTypeConsume, Quota: 200, PromptTokens: 20, CompletionTokens: 7},
		{SiteId: 0, CreatedAt: now - 10, Type: LogTypeError, Quota: 900, PromptTokens: 90, CompletionTokens: 90},
	}
	for i := range logs {
		logs[i].Username = "usage-user"
		logs[i].ModelName = "usage-model"
		logs[i].TokenName = "usage-token"
		logs[i].ChannelId = 17
		logs[i].Group = "usage-group"
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	for _, tc := range []struct {
		name   string
		siteID int
		end    int64
		want   Stat
	}{
		{name: "all sites", siteID: SiteScopeAll, want: Stat{Quota: 350, Rpm: 2, Tpm: 42}},
		{name: "main site", siteID: 0, want: Stat{Quota: 150, Rpm: 1, Tpm: 15}},
		{name: "sub-site", siteID: 7, want: Stat{Quota: 200, Rpm: 1, Tpm: 27}},
		{name: "empty site", siteID: 8, want: Stat{}},
		// Historical quota uses the requested time range, while rates use the last minute.
		{name: "historical quota", siteID: 0, end: now - 60, want: Stat{Quota: 50, Rpm: 1, Tpm: 15}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stat, err := SumUsedQuota(LogTypeConsume, now-7200, tc.end, "usage-model", "usage-user", "usage-token", 17, "usage-group", tc.siteID)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stat)
		})
	}
}
