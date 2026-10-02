package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUserValidatesRolesAndPreservesSite(t *testing.T) {
	for _, test := range []struct {
		name         string
		role         *int
		operatorRole int
		wantRole     int
		wantSuccess  bool
	}{
		{name: "negative", role: new(-1), operatorRole: common.RoleAdminUser},
		{name: "unknown below subsite admin", role: new(2), operatorRole: common.RoleAdminUser},
		{name: "unknown above subsite admin", role: new(6), operatorRole: common.RoleAdminUser},
		{name: "unknown below root", role: new(99), operatorRole: common.RoleRootUser},
		{name: "omitted defaults to common", operatorRole: common.RoleAdminUser, wantRole: common.RoleCommonUser, wantSuccess: true},
		{name: "guest preserves creation default", role: new(common.RoleGuestUser), operatorRole: common.RoleAdminUser, wantRole: common.RoleCommonUser, wantSuccess: true},
		{name: "common", role: new(common.RoleCommonUser), operatorRole: common.RoleAdminUser, wantRole: common.RoleCommonUser, wantSuccess: true},
		{name: "subsite admin remains valid", role: new(common.RoleSubSiteAdmin), operatorRole: common.RoleAdminUser, wantRole: common.RoleSubSiteAdmin, wantSuccess: true},
		{name: "root creates admin", role: new(common.RoleAdminUser), operatorRole: common.RoleRootUser, wantRole: common.RoleAdminUser, wantSuccess: true},
		{name: "admin cannot create admin", role: new(common.RoleAdminUser), operatorRole: common.RoleAdminUser},
		{name: "admin cannot create root", role: new(common.RoleRootUser), operatorRole: common.RoleAdminUser},
		{name: "root cannot create root", role: new(common.RoleRootUser), operatorRole: common.RoleRootUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupManageUserTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.ExternalIdentityClaim{}))
			require.NoError(t, i18n.Init())
			previousGiftQuota := common.QuotaForNewUser
			common.QuotaForNewUser = 0
			t.Cleanup(func() { common.QuotaForNewUser = previousGiftQuota })

			body := map[string]any{
				"username": "created-user", "password": "member-password-1", "site_id": 777,
			}
			if test.role != nil {
				body["role"] = *test.role
			}
			encoded, err := common.Marshal(body)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/", bytes.NewReader(encoded))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("role", test.operatorRole)
			ctx.Set("id", 9999)
			ctx.Set("username", "creating-admin")
			common.SetContextKey(ctx, constant.ContextKeySiteId, 7)

			CreateUser(ctx)

			assert.Equal(t, http.StatusOK, recorder.Code)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
			assert.Equal(t, test.wantSuccess, result.Success, recorder.Body.String())
			var users []model.User
			require.NoError(t, db.Find(&users).Error)
			if test.wantSuccess {
				require.Len(t, users, 1)
				assert.Equal(t, test.wantRole, users[0].Role)
				assert.Equal(t, 7, users[0].SiteId, "request body must not override the resolved site")
				assert.Equal(t, "created-user", users[0].DisplayName)
				assert.True(t, common.ValidatePasswordAndHash("member-password-1", users[0].Password))
				assert.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAuditLogged))
			} else {
				assert.Empty(t, users, "rejected roles must not create accounts")
				for _, entity := range []any{&model.CasbinRule{}, &model.ExternalIdentityClaim{}, &model.Log{}} {
					var count int64
					require.NoError(t, db.Model(entity).Count(&count).Error)
					assert.Zero(t, count, "rejected roles must not create authorization, identity or success-audit records")
				}
			}
		})
	}
}
