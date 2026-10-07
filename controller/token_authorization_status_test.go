package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenWritesAuthorizeEverySelectedGroup(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldUsable, oldRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","unpriced":"No ratio"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"member":1,"hidden":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
	})
	for _, method := range []string{http.MethodPost, http.MethodPut, "ENABLE"} {
		for _, tc := range []struct {
			name, group, want string
			allowed           bool
			role              int
		}{
			{"hidden", "hidden", "", false, common.RoleCommonUser},
			{"mixed_hidden", "default,hidden", "", false, common.RoleCommonUser},
			{"unpriced", "unpriced", "", false, common.RoleCommonUser},
			{"normalized", " vip, default,vip, ", "vip,default", true, common.RoleCommonUser},
			{"actual_user_group", "member", "member", true, common.RoleCommonUser},
			{"auto", "auto", "auto", true, common.RoleCommonUser},
			{"mixed_auto", "auto,vip", "auto,vip", true, common.RoleCommonUser},
			{"admin_no_new_bypass", "hidden", "", false, common.RoleAdminUser},
			{"root_no_new_bypass", "hidden", "", false, common.RoleRootUser},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				db := setupTokenControllerTestDB(t)
				require.NoError(t, db.AutoMigrate(&model.User{}))
				user := model.User{Id: 101, Username: "actual-member", Group: "member", Role: tc.role, Status: common.UserStatusEnabled}
				require.NoError(t, db.Create(&user).Error)
				request := map[string]any{"name": "changed", "group": tc.group, "expired_time": -1, "unlimited_quota": true, "user_id": 999, "role": common.RoleRootUser, "user_group": "hidden"}
				var old *model.Token
				target := "/api/token/"
				verb := method
				if method != http.MethodPost {
					old = seedToken(t, db, user.Id, "original", "group-auth-secret")
					request["id"] = old.Id
					if method == "ENABLE" {
						verb = http.MethodPut
						target += "?status_only=true"
						old.Group = tc.group
						old.Status = common.TokenStatusDisabled
						require.NoError(t, db.Save(old).Error)
						request["status"] = common.TokenStatusEnabled
						request["group"] = "default"
					}
				}
				ctx, recorder := newAuthenticatedContext(t, verb, target, request, user.Id)
				ctx.Set("role", tc.role)
				ctx.Set("group", user.Group)
				if method == http.MethodPost {
					AddToken(ctx)
				} else {
					UpdateToken(ctx)
				}
				result := decodeAPIResponse(t, recorder)
				require.Equal(t, tc.allowed, result.Success, result.Message)
				var rows []model.Token
				require.NoError(t, db.Find(&rows).Error)
				if !tc.allowed {
					if old == nil {
						require.Empty(t, rows)
					} else {
						require.Len(t, rows, 1)
						assert.Equal(t, old.Group, rows[0].Group)
						assert.Equal(t, old.Status, rows[0].Status)
						assert.Equal(t, old.Name, rows[0].Name)
					}
					return
				}
				require.Len(t, rows, 1)
				assert.Equal(t, user.Id, rows[0].UserId)
				assert.Equal(t, tc.want, rows[0].Group)
			})
		}
	}
}

func TestTokenEffectiveStatusFiltersWholeUserDataset(t *testing.T) {
	for _, redisEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis_%t", redisEnabled), func(t *testing.T) {
			db := setupTokenControllerTestDB(t)
			oldRedis := common.RedisEnabled
			common.RedisEnabled = redisEnabled
			t.Cleanup(func() { common.RedisEnabled = oldRedis })
			now := common.GetTimestamp()
			type entry struct {
				name              string
				stored, effective int
				expiry            int64
				quota             int
				unlimited         bool
			}
			entries := []entry{
				{"unvisited-expired-a", 1, 3, now - 100, 10, false}, {"unvisited-expired-b", 1, 3, now - 100, 10, false},
				{"unvisited-expired-and-exhausted", 1, 3, now - 100, 0, false}, {"legacy-expired-repaired", 3, 3, now + 1000, 10, false},
				{"manual-disabled-expired", 2, 2, now - 100, 0, false}, {"manual-disabled-valid", 2, 2, -1, 10, false},
				{"unvisited-exhausted", 1, 4, -1, 0, false}, {"legacy-exhausted-repaired", 4, 4, -1, 10, false},
				{"enabled-never", 1, 1, -1, 10, false}, {"enabled-unlimited", 1, 1, -1, 0, true}, {"enabled-future", 1, 1, now + 1000, 10, false},
			}
			byID := map[int]entry{}
			for _, e := range entries {
				token := model.Token{UserId: 71, Name: e.name, Key: "private-token-" + e.name, Status: e.stored, ExpiredTime: e.expiry, RemainQuota: e.quota, UnlimitedQuota: e.unlimited, Group: "default"}
				require.NoError(t, db.Create(&token).Error)
				byID[token.Id] = e
			}
			other := model.Token{UserId: 72, Name: "unvisited-expired-other", Key: "other-owner-secret", Status: 1, ExpiredTime: now - 100, UnlimitedQuota: true}
			require.NoError(t, db.Create(&other).Error)
			for _, search := range []bool{false, true} {
				for _, status := range []int{0, 1, 2, 3, 4} {
					t.Run(fmt.Sprintf("search_%t_status_%d", search, status), func(t *testing.T) {
						expected := map[int]bool{}
						for id, e := range byID {
							if (status == 0 || status == e.effective) && (!search || strings.HasPrefix(e.name, "unvisited-")) {
								expected[id] = true
							}
						}
						seen := map[int]bool{}
						for page := 1; page <= ((len(expected)+1)/2)+1; page++ {
							target := fmt.Sprintf("/api/token/?p=%d&page_size=2&user_id=72", page)
							if status != 0 {
								target += fmt.Sprintf("&status=%d", status)
							}
							if search {
								target = fmt.Sprintf("/api/token/search?p=%d&page_size=2&user_id=72&keyword=unvisited-%%25", page)
								if status != 0 {
									target += fmt.Sprintf("&status=%d", status)
								}
							}
							c, w := newAuthenticatedContext(t, http.MethodGet, target, nil, 71)
							if search {
								SearchTokens(c)
							} else {
								GetAllTokens(c)
							}
							response := decodeAPIResponse(t, w)
							require.True(t, response.Success, response.Message)
							var data struct {
								Total int `json:"total"`
								Items []struct {
									ID              int    `json:"id"`
									Status          int    `json:"status"`
									EffectiveStatus int    `json:"effective_status"`
									Key             string `json:"key"`
								} `json:"items"`
							}
							require.NoError(t, common.Unmarshal(response.Data, &data))
							assert.Equal(t, len(expected), data.Total)
							require.LessOrEqual(t, len(data.Items), 2)
							for _, item := range data.Items {
								require.Contains(t, expected, item.ID)
								require.False(t, seen[item.ID])
								seen[item.ID] = true
								e := byID[item.ID]
								assert.Equal(t, e.stored, item.Status)
								assert.Equal(t, e.effective, item.EffectiveStatus)
								assert.NotContains(t, item.Key, "private-token-")
							}
						}
						assert.Equal(t, expected, seen)
					})
				}
			}
			for id, e := range byID {
				var stored model.Token
				require.NoError(t, db.First(&stored, id).Error)
				assert.Equal(t, e.stored, stored.Status)
			}
			for _, value := range []string{"0", "5", "garbage", "1,3", "-1", "", "1&status=3"} {
				for _, search := range []bool{false, true} {
					c, w := newAuthenticatedContext(t, http.MethodGet, "/api/token/?status="+value, nil, 71)
					if search {
						SearchTokens(c)
					} else {
						GetAllTokens(c)
					}
					assert.False(t, decodeAPIResponse(t, w).Success)
				}
			}
		})
	}
}

func TestTokenEnableChecksCurrentAvailabilityAndPreservesRepairIntent(t *testing.T) {
	require.NoError(t, i18n.Init())
	configureTokenAutoGroupsTest(t, "5", `["default"]`)
	for _, storedStatus := range []int{1, 2, 3, 4} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprintf("stored_%d_expired_%t", storedStatus, expired), func(t *testing.T) {
				db := setupTokenControllerTestDB(t)
				token := seedToken(t, db, 81, "repair", "repair-intent-key")
				token.Status = storedStatus
				token.UnlimitedQuota = false
				token.RemainQuota = 0
				if expired {
					token.ExpiredTime = common.GetTimestamp() - 100
					token.RemainQuota = 10
				}
				require.NoError(t, db.Save(token).Error)
				c, w := newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": token.Id, "status": 1}, 81)
				c.Set("group", "default")
				UpdateToken(c)
				assert.False(t, decodeAPIResponse(t, w).Success)
				require.NoError(t, db.First(token, token.Id).Error)
				assert.Equal(t, storedStatus, token.Status)
				c, w = newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{"id": token.Id, "name": "repaired", "status": 1, "group": "default", "expired_time": -1, "remain_quota": 10, "unlimited_quota": false}, 81)
				c.Set("group", "default")
				UpdateToken(c)
				response := decodeAPIResponse(t, w)
				require.True(t, response.Success, response.Message)
				require.NoError(t, db.First(token, token.Id).Error)
				assert.Equal(t, storedStatus, token.Status)
				assert.EqualValues(t, -1, token.ExpiredTime)
				assert.Equal(t, 10, token.RemainQuota)
				c, w = newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": token.Id, "status": 1}, 81)
				c.Set("group", "default")
				UpdateToken(c)
				response = decodeAPIResponse(t, w)
				require.True(t, response.Success, response.Message)
				require.NoError(t, db.First(token, token.Id).Error)
				assert.Equal(t, 1, token.Status)
			})
		}
	}
}
