package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `json:"use_group" gorm:"index;size:64;default:''"`
	TokenID   int    `json:"token_id" gorm:"index;default:0"`
	ChannelID int    `json:"channel_id" gorm:"index;default:0"`
	NodeName  string `json:"node_name" gorm:"index;size:64;default:''"`
	TokenUsed int64  `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

type QuotaDataLogParams struct {
	UserID    int
	Username  string
	ModelName string
	Quota     int
	CreatedAt int64
	TokenUsed int64
	UseGroup  string
	TokenID   int
	ChannelID int
	NodeName  string
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			if err := SaveQuotaDataCache(); err != nil {
				common.SysError("failed to save dashboard data: " + err.Error())
			}
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

// The gate serializes periodic/final saves without holding up new producers.
// An uncertain transaction is kept separate and must never be replayed blindly.
var quotaDataFlushGate = make(chan struct{}, 1)
var quotaDataUncertain map[string]*QuotaData
var quotaDataFlushError error

func logQuotaDataCache(quotaData *QuotaData) {
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
		quotaData.UserID,
		quotaData.Username,
		quotaData.ModelName,
		quotaData.CreatedAt,
		quotaData.UseGroup,
		quotaData.TokenID,
		quotaData.ChannelID,
		quotaData.NodeName,
	)
	count := quotaData.Count
	quota := quotaData.Quota
	tokenUsed := quotaData.TokenUsed
	cachedQuotaData, ok := CacheQuotaData[key]
	if ok {
		cachedQuotaData.Count += count
		cachedQuotaData.Quota += quota
		cachedQuotaData.TokenUsed = common.SumTokenCountsForStatistics(cachedQuotaData.TokenUsed, tokenUsed)
		quotaData = cachedQuotaData
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	createdAt := params.CreatedAt - (params.CreatedAt % 3600)
	quotaData := &QuotaData{
		UserID:    params.UserID,
		Username:  params.Username,
		ModelName: params.ModelName,
		CreatedAt: createdAt,
		UseGroup:  params.UseGroup,
		TokenID:   params.TokenID,
		ChannelID: params.ChannelID,
		NodeName:  params.NodeName,
		Count:     1,
		Quota:     params.Quota,
		TokenUsed: common.SumTokenCountsForStatistics(params.TokenUsed),
	}

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(quotaData)
}

func SaveQuotaDataCache() error {
	return SaveQuotaDataCacheContext(context.Background())
}

// SaveQuotaDataCacheContext commits a complete snapshot or retains it after a
// proven rollback. Unknown outcomes stop subsequent saves for reconciliation.
// Producers must be drained before using this as the final shutdown save.
func SaveQuotaDataCacheContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case quotaDataFlushGate <- struct{}{}:
		defer func() { <-quotaDataFlushGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if quotaDataFlushError != nil {
		return quotaDataFlushError
	}
	CacheQuotaDataLock.Lock()
	stores := make(map[string]*QuotaData, len(CacheQuotaData))
	for key, quotaData := range CacheQuotaData {
		row := *quotaData
		stores[key] = &row
	}
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()
	if len(stores) == 0 {
		return nil
	}
	batchID := common.GetUUID()
	tx := DB.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true}).Begin()
	if tx.Error != nil {
		requeueQuotaData(stores)
		return fmt.Errorf("dashboard batch %s begin: %w", batchID, tx.Error)
	}
	if err := applyQuotaDataSnapshot(tx, stores); err != nil {
		if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
			return quarantineQuotaData(batchID, stores, errors.Join(err, fmt.Errorf("rollback outcome unknown: %w", rollbackErr)))
		}
		requeueQuotaData(stores)
		return fmt.Errorf("dashboard batch %s rolled back: %w", batchID, err)
	}
	if err := tx.Commit().Error; err != nil {
		return quarantineQuotaData(batchID, stores, fmt.Errorf("commit outcome unknown: %w", err))
	}
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据，batch=%s", len(stores), batchID))
	return nil
}

func applyQuotaDataSnapshot(tx *gorm.DB, stores map[string]*QuotaData) error {
	keys := make([]string, 0, len(stores))
	for key := range stores {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		quotaData := stores[key]
		var existing QuotaData
		result := tx.Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
			Order("id").Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// GORM fills generated IDs even when a later statement rolls back.
			// Keep the queued delta immutable so its retry uses a fresh INSERT.
			row := *quotaData
			row.Id = 0
			result = tx.Create(&row)
		} else {
			// Update the selected row once, including when legacy data contains
			// duplicate dimensions. Never multiply a new delta across duplicates.
			result = tx.Model(&QuotaData{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{
				"count":      gorm.Expr("COALESCE(count, 0) + ?", quotaData.Count),
				"quota":      gorm.Expr("COALESCE(quota, 0) + ?", quotaData.Quota),
				"token_used": gorm.Expr("CASE WHEN COALESCE(token_used, 0) > ? THEN ? ELSE COALESCE(token_used, 0) + ? END", math.MaxInt64-quotaData.TokenUsed, int64(math.MaxInt64), quotaData.TokenUsed),
			})
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("dashboard row write affected %d rows, expected 1", result.RowsAffected)
		}
	}
	return nil
}

func requeueQuotaData(stores map[string]*QuotaData) {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	for _, quotaData := range stores {
		logQuotaDataCache(quotaData)
	}
}

func quarantineQuotaData(batchID string, stores map[string]*QuotaData, err error) error {
	quotaDataUncertain = stores
	quotaDataFlushError = fmt.Errorf("dashboard batch %s requires reconciliation; automatic replay disabled: %w", batchID, err)
	// This is operator evidence, not a durable exactly-once recovery journal.
	deltas, _ := common.Marshal(stores)
	common.SysError(fmt.Sprintf("%v; pending_dashboard_deltas=%s", quotaDataFlushError, deltas))
	return quotaDataFlushError
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}
