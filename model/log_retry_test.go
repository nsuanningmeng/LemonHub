package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRetryLogsStayAdminOnlyAcrossUserAndTokenQueries(t *testing.T) {
	db := setupUsageActivityTestDB(t)
	previousLogType := common.LogDatabaseType()
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		common.SetLogDatabaseType(previousLogType)
		initCol()
	})
	verifyRetryLogQueries(t, db)
}

func verifyRetryLogQueries(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousLimit := common.MaxRecentItems
	common.MaxRecentItems = 2
	t.Cleanup(func() { common.MaxRecentItems = previousLimit })
	logs := []Log{
		{UserId: 77, TokenId: 3, Type: LogTypeError, RequestId: "legacy", Content: "legacy failure"},
		{UserId: 77, TokenId: 3, Type: LogTypeError, RequestId: "success", IsRetry: true, Content: "recovered failure"},
		{UserId: 77, TokenId: 3, Type: LogTypeConsume, RequestId: "success", Content: "success"},
		{UserId: 77, TokenId: 3, Type: LogTypeError, RequestId: "failed", IsRetry: true, Content: "first failed attempt"},
		{UserId: 77, TokenId: 3, Type: LogTypeError, RequestId: "failed", Content: "terminal failure"},
		{UserId: 77, TokenId: 3, Type: LogTypeError, RequestId: "failed", IsRetry: true, Content: "last internal attempt"},
		{UserId: 77, TokenId: 4, Type: LogTypeConsume, RequestId: "other-token", Content: "other token"},
		{UserId: 78, TokenId: 5, Type: LogTypeError, RequestId: "other-user", Content: "other user"},
	}
	for i := range logs {
		logs[i].Id = 2001 + i
		logs[i].CreatedAt = int64(100 + i)
		logs[i].ModelName = "retry-model"
		logs[i].Group = "retry-group"
		logs[i].Other = `{"admin_info":{"diagnostic":"private"}}`
	}
	// An old writer leaves NULL on relational databases and the default 0 on
	// ClickHouse. Both must remain visible alongside newly classified attempts.
	require.NoError(t, db.Omit("is_retry").Create(&logs[0]).Error)
	require.NoError(t, db.Create(logs[1:]).Error)

	for _, tc := range []struct {
		name      string
		logType   int
		start     int
		requestID string
		want      []string
		total     int64
	}{
		{name: "first page", want: []string{"other token", "terminal failure"}, total: 4},
		{name: "second page", start: 2, want: []string{"success", "legacy failure"}, total: 4},
		{name: "errors", logType: LogTypeError, want: []string{"terminal failure", "legacy failure"}, total: 2},
		{name: "successful request", requestID: "success", want: []string{"success"}, total: 1},
		{name: "failed request", requestID: "failed", want: []string{"terminal failure"}, total: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			visible, total, err := GetUserLogs(77, tc.logType, 0, 0, "retry-model", "", tc.start, 2, "retry-group", tc.requestID, "")
			require.NoError(t, err)
			assert.Equal(t, tc.total, total)
			var contents []string
			for i, log := range visible {
				contents = append(contents, log.Content)
				assert.False(t, log.IsRetry)
				assert.Equal(t, tc.start+i+1, log.Id)
				assert.JSONEq(t, `{}`, log.Other)
			}
			assert.Equal(t, tc.want, contents)
		})
	}

	tokenLogs, err := GetLogByTokenId(3)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 2, "retry attempts must be filtered before the recent-log limit")
	assert.Equal(t, "terminal failure", tokenLogs[0].Content)
	assert.Equal(t, "success", tokenLogs[1].Content)
	common.MaxRecentItems = 10
	tokenLogs, err = GetLogByTokenId(3)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 3)
	assert.Equal(t, "legacy failure", tokenLogs[2].Content)
	for _, log := range tokenLogs {
		assert.Equal(t, 3, log.TokenId)
		assert.False(t, log.IsRetry)
		assert.JSONEq(t, `{}`, log.Other)
	}

	adminLogs, total, err := GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 10, 0, "", "failed", "", SiteScopeAll)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, adminLogs, 3)
	assert.Equal(t, "last internal attempt", adminLogs[0].Content)
	assert.True(t, adminLogs[0].IsRetry)
	assert.Equal(t, "terminal failure", adminLogs[1].Content)
	assert.False(t, adminLogs[1].IsRetry)
	assert.Equal(t, "first failed attempt", adminLogs[2].Content)
	assert.True(t, adminLogs[2].IsRetry)
	for _, log := range adminLogs {
		assert.JSONEq(t, `{"admin_info":{"diagnostic":"private"}}`, log.Other)
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		assert.Equal(t, []int{1, 2, 3}, []int{adminLogs[0].Id, adminLogs[1].Id, adminLogs[2].Id})
	}
}
