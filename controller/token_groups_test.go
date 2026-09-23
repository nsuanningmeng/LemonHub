package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenWritesRequireGroupSelection(t *testing.T) {
	require.NoError(t, i18n.Init())
	inputs := []struct {
		name    string
		include bool
		group   any
	}{
		{name: "omitted"},
		{name: "null", include: true},
		{name: "empty", include: true, group: ""},
		{name: "whitespace", include: true, group: " \t\n\u3000"},
		{name: "empty segments", include: true, group: " , ,\t,"},
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, input := range inputs {
			t.Run(method+"/"+input.name, func(t *testing.T) {
				db := setupTokenControllerTestDB(t)
				request := map[string]any{
					"name": "changed", "expired_time": -1, "unlimited_quota": true,
				}
				if input.include {
					request["group"] = input.group
				}
				var existing *model.Token
				if method == http.MethodPut {
					existing = seedToken(t, db, 1, "existing", "group-required-key")
					request["id"] = existing.Id
				}
				ctx, recorder := newAuthenticatedContext(t, method, "/api/token/", request, 1)
				ctx.Request.Header.Set("Accept-Language", "zh-CN")
				if method == http.MethodPost {
					AddToken(ctx)
				} else {
					UpdateToken(ctx)
				}
				response := decodeAPIResponse(t, recorder)
				assert.False(t, response.Success)
				assert.Equal(t, "请至少选择一个分组", response.Message)
				var count int64
				require.NoError(t, db.Model(&model.Token{}).Count(&count).Error)
				if existing == nil {
					assert.Zero(t, count, "rejected requests must not create keys")
					return
				}
				assert.EqualValues(t, 1, count)
				var saved model.Token
				require.NoError(t, db.First(&saved, existing.Id).Error)
				assert.Equal(t, existing.Group, saved.Group)
				assert.Equal(t, existing.Name, saved.Name)
				assert.Equal(t, existing.Key, saved.Key)
			})
		}
	}
}

func TestAddTokenNormalizesExplicitGroupsAndEnforcesLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		group   string
		want    string
		success bool
	}{
		{name: "ordered groups", group: " , vip, default ,vip,", want: "vip,default", success: true},
		{name: "explicit auto", group: " ,auto,auto, ", want: "auto", success: true},
		{name: "at limit", group: "a,b,c,d,e,f,g,h", want: "a,b,c,d,e,f,g,h", success: true},
		{name: "over limit", group: "a,b,c,d,e,f,g,h,i"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupTokenControllerTestDB(t)
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
				"name": test.name, "group": test.group, "expired_time": -1,
				"unlimited_quota": true, "cross_group_retry": true,
			}, 1)
			AddToken(ctx)
			response := decodeAPIResponse(t, recorder)
			require.Equal(t, test.success, response.Success, response.Message)
			var saved []model.Token
			require.NoError(t, db.Find(&saved).Error)
			if !test.success {
				assert.Empty(t, saved)
				return
			}
			require.Len(t, saved, 1)
			assert.Equal(t, test.want, saved[0].Group)
			assert.Equal(t, test.want == "auto", saved[0].CrossGroupRetry)
		})
	}
}

func TestLegacyTokenCanBeDisabledAndRepairedBeforeEnabling(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "legacy", "legacy-group-key")
	token.Group = " , \t,"
	token.UsedQuota = 42
	require.NoError(t, db.Save(token).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusDisabled,
	}, 1)
	UpdateToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)

	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusEnabled, "group": "default",
	}, 1)
	UpdateToken(ctx)
	response = decodeAPIResponse(t, recorder)
	assert.False(t, response.Success, "status-only must validate the stored group")
	assert.Equal(t, "Please select at least one group", response.Message)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)

	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": token.Name, "group": "vip, default",
		"expired_time": token.ExpiredTime, "remain_quota": token.RemainQuota,
		"unlimited_quota": token.UnlimitedQuota,
	}, 1)
	UpdateToken(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, "vip,default", token.Group)
	assert.Equal(t, "legacy-group-key", token.Key)
	assert.Equal(t, 42, token.UsedQuota)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	assert.False(t, strings.Contains(recorder.Body.String(), token.Key))

	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusEnabled,
	}, 1)
	UpdateToken(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NoError(t, db.First(token, token.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
}
