package openai

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func realtimeShutdownWebsocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	type upgradeResult struct {
		conn *websocket.Conn
		err  error
	}
	accepted := make(chan upgradeResult, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		accepted <- upgradeResult{conn: conn, err: err}
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	var peer *websocket.Conn
	select {
	case result := <-accepted:
		require.NoError(t, result.err)
		peer = result.conn
	case <-time.After(2 * time.Second):
		t.Fatal("websocket upgrade did not finish")
	}
	t.Cleanup(func() { _ = peer.Close() })
	return client, peer
}

func TestRealtimeDisconnectWaitsForReaderBillingAndClosesPeer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "realtime.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	savedDB, savedLogDB := model.DB, model.LOG_DB
	savedRedis, savedBatch := common.RedisEnabled, common.BatchUpdateEnabled
	savedDatabaseType, savedLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = savedDB, savedLogDB
		common.RedisEnabled, common.BatchUpdateEnabled = savedRedis, savedBatch
		common.SetDatabaseTypes(savedDatabaseType, savedLogDatabaseType)
		assert.NoError(t, sqlDB.Close())
	})
	// The normal startup also initializes quoted SQL column names used when
	// realtime billing loads its token. Reuse that initialization for this DB.
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	savedModelRatio := ratio_setting.ModelRatio2JSONString()
	savedGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		assert.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedModelRatio))
		assert.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroupRatio))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-realtime-shutdown":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"test-realtime-shutdown":1}`))
	require.NoError(t, db.Create(&model.User{Id: 15101, Username: "realtime_shutdown", Quota: 100}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 15102, UserId: 15101, Key: "realtime-shutdown-key", RemainQuota: 100}).Error)

	client, clientRelay := realtimeShutdownWebsocketPair(t)
	targetRelay, upstream := realtimeShutdownWebsocketPair(t)
	enteredBilling := make(chan struct{})
	releaseBilling := make(chan struct{})
	var blockOnce, releaseOnce sync.Once
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:block_realtime_billing", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			blockOnce.Do(func() {
				close(enteredBilling)
				<-releaseBilling
			})
		}
	}))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{
		UserId:          15101,
		TokenId:         15102,
		TokenKey:        "realtime-shutdown-key",
		OriginModelName: "test-realtime-shutdown",
		UsingGroup:      "test-realtime-shutdown",
		UserGroup:       "test-realtime-shutdown",
		ClientWs:        clientRelay,
		TargetWs:        targetRelay,
	}
	type handlerResult struct {
		err   *types.NewAPIError
		usage *dto.RealtimeUsage
	}
	results := make(chan handlerResult, 1)
	finished := make(chan struct{})
	go func() {
		err, usage := OpenaiRealtimeHandler(c, info)
		results <- handlerResult{err: err, usage: usage}
		close(finished)
	}()
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseBilling) })
		_ = client.Close()
		_ = upstream.Close()
		_ = clientRelay.Close()
		_ = targetRelay.Close()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("realtime handler did not finish cleanup")
		}
	})

	body, err := common.Marshal(dto.RealtimeEvent{
		Type: dto.RealtimeEventTypeResponseDone,
		Response: &dto.RealtimeResponse{Usage: &dto.RealtimeUsage{
			TotalTokens: 10,
			InputTokens: 10,
			InputTokenDetails: dto.InputTokenDetails{
				TextTokens: 10,
			},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, upstream.WriteMessage(websocket.TextMessage, body))
	select {
	case <-enteredBilling:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream usage did not reach billing")
	}

	require.NoError(t, client.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(2*time.Second)))
	require.NoError(t, client.Close())
	require.NoError(t, upstream.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = upstream.ReadMessage()
	require.Error(t, err, "disconnect must close the other socket while its reader is settling usage")
	if netErr, ok := err.(interface{ Timeout() bool }); ok {
		require.False(t, netErr.Timeout(), "handler left the upstream socket open")
	}
	select {
	case <-finished:
		t.Fatal("handler returned before its reader finished billing")
	default:
	}
	releaseOnce.Do(func() { close(releaseBilling) })
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("handler failed to finish after billing completed")
	}
	result := <-results
	require.Nil(t, result.err)
	require.NotNil(t, result.usage)
	assert.Equal(t, 10, result.usage.TotalTokens)
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, 15101).Error)
	require.NoError(t, db.First(&token, 15102).Error)
	assert.Equal(t, 90, user.Quota, "usage must be charged exactly once across disconnect")
	assert.Equal(t, 90, token.RemainQuota)
	assert.Equal(t, 10, token.UsedQuota)
}
