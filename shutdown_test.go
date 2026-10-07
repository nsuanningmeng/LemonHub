package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupShutdownDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "shutdown.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}))
	oldDB := model.DB
	oldRedis, oldBatch, oldExport := common.RedisEnabled, common.BatchUpdateEnabled, common.DataExportEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	common.RedisEnabled, common.BatchUpdateEnabled, common.DataExportEnabled = false, true, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = oldDB
		common.RedisEnabled, common.BatchUpdateEnabled, common.DataExportEnabled = oldRedis, oldBatch, oldExport
		common.SetDatabaseTypes(oldMainType, oldLogType)
		assert.NoError(t, sqlDB.Close())
	})
	return db
}

func TestShutdownAccountingClosesIdleWebSocketAndPersistsHandlerTail(t *testing.T) {
	db := setupShutdownDatabase(t)
	channel := model.Channel{Name: "shutdown-idle-websocket", Key: "test"}
	require.NoError(t, db.Create(&channel).Error)
	upgraded := make(chan error, 1)
	requests := newDrainingHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		upgraded <- err
		if err != nil {
			return
		}
		defer conn.Close()
		defer model.UpdateChannelUsedQuota(channel.Id, 5)
		_, _, _ = conn.ReadMessage()
	}))
	server := httptest.NewServer(requests)
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, <-upgraded)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, shutdownAccounting(ctx, server.Config, requests))
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Equal(t, int64(5), channel.UsedQuota, "hijacked handler settlement must finish before final flush")
	_, _, err = client.ReadMessage()
	require.Error(t, err, "shutdown must close an idle hijacked socket")
	response := httptest.NewRecorder()
	requests.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestDrainingHandlerPreservesGinStreamAndResponseController(t *testing.T) {
	router := gin.New()
	controllerResult := make(chan error, 1)
	router.GET("/stream", func(c *gin.Context) {
		controllerResult <- http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(time.Second))
		c.Stream(func(w io.Writer) bool {
			_, _ = io.WriteString(w, "data: final\n\n")
			return false
		})
	})
	requests := newDrainingHandler(router)
	server := httptest.NewServer(requests)
	t.Cleanup(server.Close)
	response, err := server.Client().Get(server.URL + "/stream")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "data: final\n\n", string(body))
	// Deadline forwarding and Gin streaming must retain their underlying
	// ResponseController, CloseNotify, and Flush support.
	require.NoError(t, <-controllerResult)
	requests.stop()
	require.NoError(t, requests.wait(context.Background()))
	underlying := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &drainingResponseWriter{ResponseWriter: underlying, request: &drainingRequest{ctx: ctx}}
	require.NoError(t, http.NewResponseController(writer).Flush())
	assert.True(t, underlying.Flushed)
	assert.ErrorIs(t, writer.Push("/asset", nil), http.ErrNotSupported)
	notify := writer.CloseNotify()
	cancel()
	select {
	case <-notify:
	case <-time.After(time.Second):
		t.Fatal("fallback CloseNotify did not observe request cancellation")
	}
}

func TestShutdownAccountingFlushesHealthyTailWhenRefundFailed(t *testing.T) {
	// Refund failure is intentionally sticky for the lifetime of a process.
	// Isolate that state so this regression is repeatable with -count and does
	// not turn unrelated graceful-shutdown tests into failed reconciliations.
	if os.Getenv("LEMONHUB_TEST_SHUTDOWN_REFUND_FAILURE") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownAccountingFlushesHealthyTailWhenRefundFailed$", "-test.v")
		cmd.Env = append(os.Environ(), "LEMONHUB_TEST_SHUTDOWN_REFUND_FAILURE=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	db := setupShutdownDatabase(t)
	owner := model.User{Username: "shutdown-refund", AffCode: "shutdown-refund", Quota: 1000}
	require.NoError(t, db.Create(&owner).Error)
	token := model.Token{UserId: owner.Id, Key: "shutdown-refund", RemainQuota: 1000}
	require.NoError(t, db.Create(&token).Error)
	channel := model.Channel{Name: "healthy-shutdown-tail", Key: "test"}
	require.NoError(t, db.Create(&channel).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{UserId: owner.Id, TokenId: token.Id, TokenKey: token.Key, ForcePreConsume: true, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	session, apiErr := service.NewBillingSession(c, info, 13)
	require.Nil(t, apiErr)
	forced := errors.New("refund persistence rejected")
	const callback = "test:shutdown-refund-rejected"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(forced)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callback) })
	session.Refund(c)
	model.UpdateChannelUsedQuota(channel.Id, 11)
	requests := newDrainingHandler(http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := shutdownAccounting(ctx, &http.Server{Handler: requests}, requests)
	require.ErrorIs(t, err, service.ErrBillingRefundsFailed)
	require.ErrorIs(t, err, forced)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Equal(t, int64(11), channel.UsedQuota, "a failed refund must not discard unrelated healthy accounting")
	require.NoError(t, db.First(&owner, owner.Id).Error)
	assert.Equal(t, 987, owner.Quota)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, 1000, token.RemainQuota)
}
