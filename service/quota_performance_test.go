package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Redis recording follows the in-memory update. Wait for that observable
// completion instead of polling or sleeping for the asynchronous recorder.
type performanceRecordHook struct {
	prefix string
	done   chan error
}

var realtimePerformanceChannelID atomic.Int64

func (h *performanceRecordHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h *performanceRecordHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (h *performanceRecordHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h *performanceRecordHook) AfterProcessPipeline(_ context.Context, commands []redis.Cmder) error {
	for _, command := range commands {
		args := command.Args()
		if command.Name() == "hincrby" && len(args) > 1 && strings.HasPrefix(fmt.Sprint(args[1]), h.prefix) {
			h.done <- command.Err()
			break
		}
	}
	return nil
}

func TestPostWssConsumeQuotaRecordsSuccessfulPerformanceOnce(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousSQLitePath, previousMaster := common.SQLitePath, common.IsMasterNode
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.SQLitePath, common.IsMasterNode = previousSQLitePath, previousMaster
	})
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	common.IsMasterNode = false
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Log{}, &model.PerfMetric{}))
	// A fresh channel isolates retained hot samples when the test is repeated.
	id := int(realtimePerformanceChannelID.Add(1))
	modelName := t.Name()
	seedUser(t, id, 1000)
	seedChannel(t, id)
	require.NoError(t, model.DB.Create(&model.PerfMetric{
		ChannelID: id, ModelName: modelName, Group: "default",
		BucketTs: time.Now().Add(-time.Hour).Unix(), RequestCount: 1,
	}).Error)
	previousEnabled := perf_metrics_setting.GetSetting().Enabled
	previousRedisEnabled, previousRedis := common.RedisEnabled, common.RDB
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.enabled": "true"}))
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	hook := &performanceRecordHook{
		prefix: fmt.Sprintf("channel-perf:%d:%s:default:", id, modelName),
		done:   make(chan error, 1),
	}
	client.AddHook(hook)
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRedis
		require.NoError(t, client.Close())
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"perf_metrics_setting.enabled": strconv.FormatBool(previousEnabled),
		}))
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	started := time.Now().Add(-time.Second)
	info := &relaycommon.RelayInfo{
		UserId: id, OriginModelName: modelName, UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: id},
		StartTime:   started, FirstResponseTime: started.Add(100 * time.Millisecond), IsStream: true,
	}
	usage := &dto.RealtimeUsage{
		InputTokens: 2, OutputTokens: 8, TotalTokens: 10,
		InputTokenDetails:  dto.InputTokenDetails{TextTokens: 2},
		OutputTokenDetails: dto.OutputTokenDetails{TextTokens: 8},
	}
	PostWssConsumeQuota(ctx, info, modelName, usage, "")
	select {
	case err := <-hook.done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("successful realtime request did not record performance")
	}

	result, err := perfmetrics.QuerySummaryAll(24, map[string][]string{modelName: {"default"}})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, int64(2), result.Models[0].RequestCount, "one prior failure and exactly one realtime success")
	assert.Equal(t, 50.0, result.Models[0].SuccessRate)
	detail, err := perfmetrics.Query(perfmetrics.QueryParams{Model: modelName, Group: "default", Hours: 24})
	require.NoError(t, err)
	require.Len(t, detail.Groups, 1)
	assert.Equal(t, int64(100), detail.Groups[0].AvgTtftMs)
	assert.Positive(t, detail.Groups[0].AvgTps)
}
