package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoProxyUsesTaskCredentialSnapshotAndLegacySingleKey(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	setting := system_setting.GetFetchSetting()
	oldSetting := *setting
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
		*setting = oldSetting
		service.InitHttpClient()
	})
	common.MemoryCacheEnabled = false
	setting.EnableSSRFProtection = false
	service.InitHttpClient()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	for index, tc := range []struct {
		name                 string
		kind                 int
		snapshot, channelKey string
		multi                bool
		wantKey              string
		wantStatus           int
	}{
		{"sora_snapshot_reordered", constant.ChannelTypeSora, "chosen-A", "chosen-B\nchosen-A", true, "chosen-A", 200},
		{"sora_legacy_single", constant.ChannelTypeSora, "", "legacy-A", false, "legacy-A", 200},
		{"sora_legacy_multi_unknown", constant.ChannelTypeSora, "", "chosen-B\nchosen-A", true, "", 500},
		{"gemini_snapshot_reordered", constant.ChannelTypeGemini, "chosen-A", "chosen-B\nchosen-A", true, "chosen-A", 200},
		{"gemini_legacy_single", constant.ChannelTypeGemini, "", "legacy-A", false, "legacy-A", 200},
		{"gemini_legacy_multi_unknown", constant.ChannelTypeGemini, "", "chosen-B\nchosen-A", true, "", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			seen := ""
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.kind == constant.ChannelTypeGemini {
					seen = r.Header.Get("x-goog-api-key")
					assert.Empty(t, r.URL.Query().Get("key"), "credentials travel only in the provider header")
				} else {
					seen = r.Header.Get("Authorization")
				}
				w.Header().Set("Content-Type", "video/mp4")
				w.Write([]byte("video"))
			}))
			defer upstream.Close()
			ch := model.Channel{Id: index + 100, Type: tc.kind, Key: tc.channelKey, BaseURL: common.GetPointer(upstream.URL), ChannelInfo: model.ChannelInfo{IsMultiKey: tc.multi}}
			require.NoError(t, db.Create(&ch).Error)
			task := model.Task{TaskID: tc.name, UserId: 7, ChannelId: ch.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: tc.snapshot, UpstreamTaskID: "operation", ResultURL: upstream.URL}}
			if tc.kind == constant.ChannelTypeGemini {
				task.Data = []byte(`{"uri":"` + upstream.URL + `"}`)
			}
			require.NoError(t, db.Create(&task).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/v1/videos/"+task.TaskID+"/content", nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 7)
			VideoProxy(c)
			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus == 200 {
				require.Equal(t, 1, calls)
				if tc.kind == constant.ChannelTypeGemini {
					require.Equal(t, tc.wantKey, seen)
				} else {
					require.Equal(t, "Bearer "+tc.wantKey, seen)
				}
				require.Equal(t, "video", w.Body.String())
			} else {
				require.Zero(t, calls)
			}
			require.NotContains(t, w.Body.String(), "chosen-A")
			require.NotContains(t, w.Body.String(), "chosen-B")
		})
	}
}

type taskCredentialRoundTrip func(*http.Request) (*http.Response, error)

func (f taskCredentialRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVertexTaskVideoResolutionPreservesSnapshotAndLegacyPrettyJSON(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	t.Cleanup(func() { model.DB = oldDB; common.MemoryCacheEnabled = oldCache })
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encodedKey, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey}))
	credentials, err := common.Marshal(map[string]string{"project_id": "selected-project", "client_email": "chosen@example.invalid", "private_key": pemKey})
	require.NoError(t, err)
	var pretty bytes.Buffer
	require.NoError(t, json.Indent(&pretty, credentials, "", "  "))
	// OAuth exchanges deliberately use a separate verified client. Intercept
	// its network boundary with a trusted Google-host certificate, keeping the
	// real signing, HTTPS exchange, and task credential selection in this test.
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "www.googleapis.com"}, DNSNames: []string{"www.googleapis.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &rsaKey.PublicKey, rsaKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	var oauthCalls atomic.Int32
	oauth := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oauthCalls.Add(1)
		if !assert.NotNil(t, r.TLS) ||
			!assert.Equal(t, http.MethodPost, r.Method) ||
			!assert.Equal(t, "/oauth2/v4/token", r.URL.Path) ||
			!assert.Equal(t, "www.googleapis.com", r.Host) ||
			!assert.NoError(t, r.ParseForm()) ||
			!assert.Equal(t, "urn:ietf:params:oauth:grant-type:jwt-bearer", r.Form.Get("grant_type")) ||
			!assert.NotEmpty(t, r.Form.Get("assertion")) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"local-token","expires_in":3600}`)
	}))
	oauth.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{certDER}, PrivateKey: rsaKey}}}
	oauth.StartTLS()
	t.Cleanup(oauth.Close)
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if !assert.Equal(t, "www.googleapis.com:443", address, "the fixture must never contact a real OAuth endpoint") {
			return nil, fmt.Errorf("unexpected OAuth dial target: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, oauth.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
	for index, tc := range []struct {
		name, snapshot, key string
		multi               bool
		wantCalls           int
	}{
		{"snapshot_reordered", pretty.String(), `[{"project_id":"other"},` + string(credentials) + `]`, true, 2},
		{"legacy_single_prettyJSON", "", pretty.String(), false, 2},
		{"legacy_multi_unknown", "", `[` + string(credentials) + `,{"project_id":"other"}]`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			beforeOAuth := oauthCalls.Load()
			client.Transport = taskCredentialRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "local.invalid", r.URL.Host)
				require.Equal(t, "Bearer local-token", r.Header.Get("Authorization"))
				require.Equal(t, "selected-project", r.Header.Get("x-goog-user-project"))
				body := `{"done":true,"response":{"videos":[{"mimeType":"video/mp4","bytesBase64Encoded":"dmlkZW8="}]}}`
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			ch := &model.Channel{Id: index + 1100, Type: constant.ChannelTypeVertexAi, Key: tc.key, BaseURL: common.GetPointer("http://local.invalid"), ChannelInfo: model.ChannelInfo{IsMultiKey: tc.multi}}
			require.NoError(t, db.Create(ch).Error)
			task := &model.Task{TaskID: tc.name, UserId: 7, ChannelId: ch.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: tc.snapshot, UpstreamTaskID: taskcommon.EncodeLocalTaskID("projects/selected-project/locations/us-central1/publishers/google/models/veo-3/operations/test")}}
			require.NoError(t, db.Create(task).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/v1/videos/"+task.TaskID+"/content", nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 7)
			VideoProxy(c)
			require.Equal(t, tc.wantCalls/2, calls, "operation fetches")
			require.Equal(t, int32(tc.wantCalls/2), oauthCalls.Load()-beforeOAuth, "verified OAuth exchanges")
			if tc.wantCalls == 0 {
				require.Equal(t, 500, w.Code)
				require.NotContains(t, w.Body.String(), "private_key")
			} else {
				require.Equal(t, 200, w.Code)
				require.Equal(t, "video", w.Body.String())
			}

		})
	}
}

func TestGeminiVideoProxyErrorsDoNotLeakTaskCredential(t *testing.T) {
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	setting := system_setting.GetFetchSetting()
	oldSetting := *setting
	common.LogWriterMu.Lock()
	oldLog := gin.DefaultErrorWriter
	var logs bytes.Buffer
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
		*setting = oldSetting
		service.InitHttpClient()
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldLog
		common.LogWriterMu.Unlock()
	})
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer upstream.Close()
	for i, tc := range []struct {
		name, url string
		blocked   bool
		status    int
	}{
		{"invalid", "http://%zz", false, 500},
		{"blocked", upstream.URL, true, 403},
		{"http_error", upstream.URL, false, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.EnableSSRFProtection = tc.blocked
			setting.AllowPrivateIp = false
			service.InitHttpClient()
			ch := model.Channel{Id: 900 + i, Type: constant.ChannelTypeGemini, Key: "other", BaseURL: common.GetPointer(upstream.URL)}
			require.NoError(t, db.Create(&ch).Error)
			task := model.Task{TaskID: tc.name, UserId: 7, ChannelId: ch.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: "SENSITIVE_SELECTED_KEY"}, Data: []byte(`{"uri":"` + tc.url + `"}`)}
			require.NoError(t, db.Create(&task).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/v1/videos/"+task.TaskID+"/content", nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 7)
			VideoProxy(c)
			require.Equal(t, tc.status, w.Code)
			require.NotContains(t, w.Body.String(), "SENSITIVE_SELECTED_KEY")
			require.NotContains(t, logs.String(), "SENSITIVE_SELECTED_KEY")
		})
	}
}
