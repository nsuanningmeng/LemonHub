package controller

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSharedVerification(t *testing.T) (*gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ExternalIdentityClaim{}, &model.EmailSuppression{}, &model.Token{}))
	require.NoError(t, i18n.Init())
	server := miniredis.RunT(t)
	previousClient := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	common.RedisEnabled = true
	previousRegister, previousPassword, previousVerify := common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled
	previousGift, previousToken := common.QuotaForNewUser, constant.GenerateDefaultToken
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = true, true, true
	common.QuotaForNewUser, constant.GenerateDefaultToken = 0, false
	t.Cleanup(func() {
		common.RDB.Close()
		common.RDB = previousClient
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = previousRegister, previousPassword, previousVerify
		common.QuotaForNewUser, constant.GenerateDefaultToken = previousGift, previousToken
	})
	return db, server
}

func verificationRequest(handler gin.HandlerFunc, siteID, userID int, target, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeySiteId, siteID)
	c.Set("id", userID)
	handler(c)
	return recorder
}

func verificationResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	return body
}

type verificationSMTP struct {
	listener net.Listener
	messages chan string
	gate     chan struct{}
	accepts  atomic.Int64
	wg       sync.WaitGroup
}

// All delivery in these tests terminates at this local SMTP server. When gated,
// the first DATA response fails only after the next send has completed.
func startVerificationSMTP(t *testing.T, gated bool) *verificationSMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &verificationSMTP{listener: listener, messages: make(chan string, 4)}
	if gated {
		server.gate = make(chan struct{})
	}
	previous := common.SMTPConfig{
		Server: common.SMTPServer, Port: common.SMTPPort, From: common.SMTPFrom,
		Account: common.SMTPAccount, Token: common.SMTPToken,
		SSLEnabled: common.SMTPSSLEnabled, StartTLSEnabled: common.SMTPStartTLSEnabled,
		ForceAuthLogin: common.SMTPForceAuthLogin,
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	common.SMTPServer, common.SMTPPort = host, portNumber
	common.SMTPFrom, common.SMTPAccount, common.SMTPToken = "sender@example.test", "", ""
	common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPForceAuthLogin = false, false, false
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			sequence := server.accepts.Add(1)
			server.wg.Add(1)
			go func() {
				defer server.wg.Done()
				defer connection.Close()
				reader := textproto.NewReader(bufio.NewReader(connection))
				fmt.Fprint(connection, "220 localhost ESMTP\r\n")
				for {
					line, err := reader.ReadLine()
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"), strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						fmt.Fprint(connection, "250 localhost\r\n")
					case line == "DATA":
						fmt.Fprint(connection, "354 continue\r\n")
						message, err := reader.ReadDotBytes()
						if err != nil {
							return
						}
						server.messages <- string(message)
						if sequence == 1 && server.gate != nil {
							<-server.gate
							fmt.Fprint(connection, "451 delivery rejected\r\n")
						} else {
							fmt.Fprint(connection, "250 accepted\r\n")
						}
					case line == "QUIT":
						fmt.Fprint(connection, "221 bye\r\n")
						return
					default:
						fmt.Fprint(connection, "500 unsupported\r\n")
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		if server.gate != nil {
			select {
			case <-server.gate:
			default:
				close(server.gate)
			}
		}
		server.wg.Wait()
		common.SMTPServer, common.SMTPPort, common.SMTPFrom = previous.Server, previous.Port, previous.From
		common.SMTPAccount, common.SMTPToken = previous.Account, previous.Token
		common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPForceAuthLogin = previous.SSLEnabled, previous.StartTLSEnabled, previous.ForceAuthLogin
	})
	return server
}

func TestSharedVerificationRegistrationAndEmailBinding(t *testing.T) {
	db, server := setupSharedVerification(t)
	smtp := startVerificationSMTP(t, false)
	previousRestricted, previousDomains := common.EmailDomainRestrictionEnabled, common.EmailDomainWhitelist
	common.EmailDomainRestrictionEnabled, common.EmailDomainWhitelist = true, []string{"EXAMPLE.TEST"}
	t.Cleanup(func() {
		common.EmailDomainRestrictionEnabled, common.EmailDomainWhitelist = previousRestricted, previousDomains
	})
	response := verificationRequest(SendEmailVerification, 7, 0, "/verify?email=New%40Example.TEST", "")
	require.Equal(t, true, verificationResponse(t, response)["success"])
	message := <-smtp.messages
	match := regexp.MustCompile(`<strong>([^<]+)</strong>`).FindStringSubmatch(message)
	require.Len(t, match, 2)
	code := match[1]
	// A separate client represents the node handling the registration request.
	firstClient := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { firstClient.Close() })
	body := fmt.Sprintf(`{"username":"registered","password":"password-123","email":"new@example.test","verification_code":%q}`, code)
	response = verificationRequest(Register, 8, 0, "/register", body)
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	response = verificationRequest(Register, 7, 0, "/register", body)
	require.Equal(t, true, verificationResponse(t, response)["success"], response.Body.String())
	var registered model.User
	require.NoError(t, db.Where("username = ? AND site_id = ?", "registered", 7).First(&registered).Error)
	assert.Equal(t, "new@example.test", registered.Email)
	response = verificationRequest(Register, 7, 0, "/register", strings.Replace(body, "registered", "replay", 1))
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	var replayCount int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "replay").Count(&replayCount).Error)
	assert.Zero(t, replayCount)

	bindUser := model.User{Username: "bind-user", Password: "hashed", SiteId: 7, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&bindUser).Error)
	require.NoError(t, common.RegisterVerificationCodeWithKey(emailVerificationKey("bound@example.test", 7), "bind", common.EmailVerificationPurpose))
	bindBody := `{"email":"BOUND@example.test","code":"bind"}`
	response = verificationRequest(EmailBind, 8, bindUser.Id, "/email/bind", bindBody)
	assert.Equal(t, http.StatusForbidden, response.Code)
	response = verificationRequest(EmailBind, 7, bindUser.Id, "/email/bind", bindBody)
	require.Equal(t, true, verificationResponse(t, response)["success"], response.Body.String())
	require.NoError(t, db.First(&bindUser, bindUser.Id).Error)
	assert.Equal(t, "bound@example.test", bindUser.Email)
	response = verificationRequest(EmailBind, 7, bindUser.Id, "/email/bind", bindBody)
	assert.Equal(t, false, verificationResponse(t, response)["success"])
}

func TestSharedVerificationResetIsSiteScopedAndConsumedBeforeBusinessFailure(t *testing.T) {
	db, _ := setupSharedVerification(t)
	smtp := startVerificationSMTP(t, false)
	for _, siteID := range []int{7, 8} {
		require.NoError(t, db.Create(&model.User{Username: "reset-user", Password: "old-hash", Email: "reset@example.test", SiteId: siteID, AffCode: fmt.Sprintf("reset-%d", siteID), Status: common.UserStatusEnabled}).Error)
	}
	key := passwordResetVerificationKey("reset@example.test", 7)
	response := verificationRequest(SendPasswordResetEmail, 7, 0, "/reset/send?email=Reset%40Example.TEST", "")
	require.Equal(t, true, verificationResponse(t, response)["success"])
	message := <-smtp.messages
	link := regexp.MustCompile(`href='([^']+)'`).FindStringSubmatch(message)
	require.Len(t, link, 2)
	parsed, err := url.Parse(link[1])
	require.NoError(t, err)
	code := parsed.Query().Get("token")
	require.NotEmpty(t, code)
	verified, err := common.VerifyCodeWithKey(key, code, common.EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified, "password reset tokens must not authorize email verification")
	body := fmt.Sprintf(`{"email":"Reset@Example.TEST","token":%q}`, code)
	response = verificationRequest(ResetPassword, 8, 0, "/reset", body)
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	response = verificationRequest(ResetPassword, 7, 0, "/reset", body)
	require.Equal(t, true, verificationResponse(t, response)["success"], response.Body.String())
	var untouched model.User
	require.NoError(t, db.Where("site_id = ?", 8).First(&untouched).Error)
	assert.Equal(t, "old-hash", untouched.Password)
	response = verificationRequest(ResetPassword, 7, 0, "/reset", body)
	assert.Equal(t, false, verificationResponse(t, response)["success"])

	require.NoError(t, common.RegisterVerificationCodeWithKey(key, "db-failure", common.PasswordResetPurpose))
	callback := "verification:reject_password_update"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("forced update failure"))
		}
	}))
	t.Cleanup(func() { db.Callback().Update().Remove(callback) })
	response = verificationRequest(ResetPassword, 7, 0, "/reset", `{"email":"reset@example.test","token":"db-failure"}`)
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	verified, err = common.VerifyCodeWithKey(key, "db-failure", common.PasswordResetPurpose)
	require.NoError(t, err)
	assert.False(t, verified, "a verified token stays consumed after downstream DB failure")
}

func TestSharedVerificationRegisterAndBindDBFailureDoesNotRestoreCode(t *testing.T) {
	for _, operation := range []string{"register", "bind"} {
		t.Run(operation, func(t *testing.T) {
			db, _ := setupSharedVerification(t)
			user := model.User{Username: "bind-db-failure", Password: "old", SiteId: 7}
			require.NoError(t, db.Create(&user).Error)
			key := emailVerificationKey("failure@example.test", 7)
			require.NoError(t, common.RegisterVerificationCodeWithKey(key, "verified", common.EmailVerificationPurpose))
			callback := "verification:reject_" + operation
			reject := func(tx *gorm.DB) {
				if tx.Statement.Table == "users" {
					tx.AddError(errors.New("forced business write failure"))
				}
			}
			var response *httptest.ResponseRecorder
			if operation == "register" {
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, reject))
				t.Cleanup(func() { db.Callback().Create().Remove(callback) })
				response = verificationRequest(Register, 7, 0, "/register", `{"username":"failure-new","password":"password-123","email":"failure@example.test","verification_code":"verified"}`)
			} else {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, reject))
				t.Cleanup(func() { db.Callback().Update().Remove(callback) })
				response = verificationRequest(EmailBind, 7, user.Id, "/bind", `{"email":"failure@example.test","code":"verified"}`)
			}
			assert.Equal(t, false, verificationResponse(t, response)["success"])
			verified, err := common.VerifyCodeWithKey(key, "verified", common.EmailVerificationPurpose)
			require.NoError(t, err)
			assert.False(t, verified)
			var changed int64
			require.NoError(t, db.Model(&model.User{}).Where("email = ?", "failure@example.test").Count(&changed).Error)
			assert.Zero(t, changed)
		})
	}
}

func TestSharedVerificationStorageFailurePreventsMailAndAccountMutation(t *testing.T) {
	db, server := setupSharedVerification(t)
	smtp := startVerificationSMTP(t, false)
	user := model.User{Username: "existing", Password: "old-hash", Email: "existing@example.test", SiteId: 7}
	require.NoError(t, db.Create(&user).Error)
	server.SetError("ERR forced storage failure")
	response := verificationRequest(SendEmailVerification, 7, 0, "/verify?email=new@example.test", "")
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	assert.Contains(t, response.Body.String(), common.ErrVerificationStoreUnavailable.Error())
	known := verificationRequest(SendPasswordResetEmail, 7, 0, "/reset/send?email=existing@example.test", "")
	unknown := verificationRequest(SendPasswordResetEmail, 7, 0, "/reset/send?email=absent@example.test", "")
	assert.Equal(t, known.Body.String(), unknown.Body.String(), "storage errors must not reveal account existence")
	assert.Equal(t, true, verificationResponse(t, known)["success"])
	assert.Zero(t, smtp.accepts.Load(), "unconfirmed code storage must never initiate delivery")
	response = verificationRequest(ResetPassword, 7, 0, "/reset", `{"email":"existing@example.test","token":"anything"}`)
	assert.Equal(t, false, verificationResponse(t, response)["success"])
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, "old-hash", user.Password)
}

func TestSharedVerificationFailedOlderDeliveryPreservesResend(t *testing.T) {
	setupSharedVerification(t)
	smtp := startVerificationSMTP(t, true)
	firstResult := make(chan *httptest.ResponseRecorder, 1)
	target := "/verify?email=" + url.QueryEscape("resend@example.test")
	go func() { firstResult <- verificationRequest(SendEmailVerification, 7, 0, target, "") }()
	firstMessage := <-smtp.messages
	response := verificationRequest(SendEmailVerification, 7, 0, target, "")
	require.Equal(t, true, verificationResponse(t, response)["success"])
	secondMessage := <-smtp.messages
	close(smtp.gate)
	assert.Equal(t, false, verificationResponse(t, <-firstResult)["success"])
	codePattern := regexp.MustCompile(`<strong>([^<]+)</strong>`)
	oldCode, newCode := codePattern.FindStringSubmatch(firstMessage), codePattern.FindStringSubmatch(secondMessage)
	require.Len(t, oldCode, 2)
	require.Len(t, newCode, 2)
	key := emailVerificationKey("resend@example.test", 7)
	verified, err := common.VerifyCodeWithKey(key, oldCode[1], common.EmailVerificationPurpose)
	require.NoError(t, err)
	assert.False(t, verified)
	verified, err = common.VerifyCodeWithKey(key, newCode[1], common.EmailVerificationPurpose)
	require.NoError(t, err)
	assert.True(t, verified)
}
