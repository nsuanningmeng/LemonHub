package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShutdownAccountingReportsDashboardWriteFailure(t *testing.T) {
	db := setupShutdownDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaData{}))
	common.DataExportEnabled = true
	model.CacheQuotaDataLock.Lock()
	previous := model.CacheQuotaData
	model.CacheQuotaData = make(map[string]*model.QuotaData)
	model.CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		model.CacheQuotaDataLock.Lock()
		model.CacheQuotaData = previous
		model.CacheQuotaDataLock.Unlock()
	})
	model.LogQuotaData(model.QuotaDataLogParams{UserID: 8201, Username: "shutdown", CreatedAt: 3600, Quota: 7, TokenUsed: 11})
	forced := errors.New("dashboard persistence rejected")
	const callback = "test:shutdown_dashboard_rejected"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data" {
			tx.AddError(forced)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })
	requests := newDrainingHandler(http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := shutdownAccounting(ctx, &http.Server{Handler: requests}, requests)
	require.ErrorIs(t, err, forced, "shutdown must not report a clean exit after losing dashboard writes")
	model.CacheQuotaDataLock.Lock()
	assert.Len(t, model.CacheQuotaData, 1)
	model.CacheQuotaDataLock.Unlock()
	require.NoError(t, db.Callback().Create().Remove(callback))
	require.NoError(t, model.SaveQuotaDataCache())
	var rows []model.QuotaData
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 7, rows[0].Quota)
}
