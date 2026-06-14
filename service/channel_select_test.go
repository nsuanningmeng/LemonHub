package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupChannelSelectTestDB 初始化一个内存 SQLite 库供渠道选择/失败转移测试使用。
// 借助 InitDB 在 !IsMasterNode 时「仅设置列名 + 打开连接、跳过迁移与建账号」的特性，
// 之后自行 AutoMigrate 所需的 Channel / Ability 两张表。结束时还原所有全局状态。
func setupChannelSelectTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	orig := struct {
		usingSQLite, usingMySQL, usingPG bool
		master, memCache, redis          bool
		sqlitePath                       string
		retryTimes                       int
		db                               *gorm.DB
	}{
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL,
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled,
		common.SQLitePath, common.RetryTimes, model.DB,
	}
	t.Cleanup(func() {
		if model.DB != nil {
			if sqlDB, err := model.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		common.UsingSQLite = orig.usingSQLite
		common.UsingMySQL = orig.usingMySQL
		common.UsingPostgreSQL = orig.usingPG
		common.IsMasterNode = orig.master
		common.MemoryCacheEnabled = orig.memCache
		common.RedisEnabled = orig.redis
		common.SQLitePath = orig.sqlitePath
		common.RetryTimes = orig.retryTimes
		model.DB = orig.db
	})

	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false // 走 DB 路径（model.GetChannel）以便构造确定性失败转移
	common.IsMasterNode = false       // 让 InitDB 跳过迁移与初始账号，仅设置 commonGroupCol 并打开连接
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	t.Setenv("SQL_DSN", "local")

	require.NoError(t, model.InitDB())
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}))
}

// addChannelWithAbility 为指定分组+模型创建一个启用渠道及对应 ability（单一优先级）。
func addChannelWithAbility(t *testing.T, id int, group, modelName string, priority int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     id,
		Name:   fmt.Sprintf("ch-%d", id),
		Key:    fmt.Sprintf("key-%d", id),
		Status: common.ChannelStatusEnabled,
		Type:   1,
		Models: modelName,
		Group:  group,
	}).Error)
	p := priority
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &p,
		Weight:    0,
	}).Error)
}

func newCtxWithGroups(userGroup string, groupList []string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserGroup, userGroup)
	if groupList != nil {
		common.SetContextKey(c, constant.ContextKeyTokenGroupList, groupList)
	}
	return c
}

// 多分组：最高优先级分组对该模型无可用渠道时，应跳过并命中下一优先级分组。
func TestCacheGetRandomSatisfiedChannel_MultiGroupSkipsEmptyGroup(t *testing.T) {
	setupChannelSelectTestDB(t)
	// 仅 groupB 有渠道；groupA 对模型 m 完全没有 ability。
	addChannelWithAbility(t, 2, "groupB", "m", 0)

	c := newCtxWithGroups("groupA", []string{"groupA", "groupB"})
	param := &RetryParam{Ctx: c, TokenGroup: "groupA", ModelName: "m", Retry: common.GetPointer(0)}

	ch, selectGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 2, ch.Id)
	require.Equal(t, "groupB", selectGroup)

	// 失败转移命中后，应把实际分组写入 ContextKeyAutoGroup（计费按实际命中分组）。
	autoGroup, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.True(t, ok)
	require.Equal(t, "groupB", autoGroup)
}

// 多分组：首选分组本次命中后，下一次重试应按优先级失败转移到下一个分组。
func TestCacheGetRandomSatisfiedChannel_FailoverAdvancesToNextGroupOnRetry(t *testing.T) {
	setupChannelSelectTestDB(t)
	// RetryTimes=0：每个分组在其唯一优先级用尽后，下次重试即切换下一分组。
	common.RetryTimes = 0
	addChannelWithAbility(t, 1, "groupA", "m", 0)
	addChannelWithAbility(t, 2, "groupB", "m", 0)

	c := newCtxWithGroups("groupA", []string{"groupA", "groupB"})
	param := &RetryParam{Ctx: c, TokenGroup: "groupA", ModelName: "m", Retry: common.GetPointer(0)}

	// 第 1 次尝试：命中最高优先级分组 groupA。
	ch1, g1, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, ch1)
	require.Equal(t, 1, ch1.Id)
	require.Equal(t, "groupA", g1)

	// 模拟外层 relay 重试循环推进一次（controller/relay.go 的 IncreaseRetry）。
	param.IncreaseRetry()

	// 第 2 次尝试：groupA 已耗尽 → 失败转移到 groupB。
	ch2, g2, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, ch2)
	require.Equal(t, 2, ch2.Id)
	require.Equal(t, "groupB", g2)

	autoGroup, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.True(t, ok)
	require.Equal(t, "groupB", autoGroup)
}

// 向后兼容：单一具体分组走原单分组路径，正常命中且不写 ContextKeyAutoGroup。
func TestCacheGetRandomSatisfiedChannel_SingleGroupUnchanged(t *testing.T) {
	setupChannelSelectTestDB(t)
	addChannelWithAbility(t, 1, "vip", "m", 0)

	c := newCtxWithGroups("vip", []string{"vip"})
	param := &RetryParam{Ctx: c, TokenGroup: "vip", ModelName: "m", Retry: common.GetPointer(0)}

	ch, g, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 1, ch.Id)
	require.Equal(t, "vip", g)

	_, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.False(t, ok, "单一具体分组路径不应写 ContextKeyAutoGroup")
}

// 多分组：所有优先级分组均无可用渠道时，返回 nil 渠道（由上层呈现「无可用渠道」）。
func TestCacheGetRandomSatisfiedChannel_AllGroupsEmptyReturnsNil(t *testing.T) {
	setupChannelSelectTestDB(t)
	// 不创建任何 ability。
	c := newCtxWithGroups("groupA", []string{"groupA", "groupB"})
	param := &RetryParam{Ctx: c, TokenGroup: "groupA", ModelName: "m", Retry: common.GetPointer(0)}

	ch, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Nil(t, ch)
}

// 向后兼容回归：单个 "auto" 令牌的跨分组失败转移仍由 token.CrossGroupRetry 标志控制，
// 不因「显式多分组必失败转移」的新逻辑而改变旧行为。
func TestCacheGetRandomSatisfiedChannel_AutoGroupCrossRetryGating(t *testing.T) {
	setupChannelSelectTestDB(t)
	common.RetryTimes = 0

	origAuto := setting.AutoGroups2JsonString()
	origUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(origAuto)
		_ = setting.UpdateUserUsableGroupsByJSONString(origUsable)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"d","ga":"a","gb":"b"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["ga","gb"]`))

	addChannelWithAbility(t, 1, "ga", "m", 0)
	addChannelWithAbility(t, 2, "gb", "m", 0)

	newAutoCtx := func(crossGroupRetry bool) *gin.Context {
		c := newCtxWithGroups("default", []string{"auto"})
		common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, crossGroupRetry)
		return c
	}

	t.Run("auto without cross_group_retry stays in first auto group", func(t *testing.T) {
		c := newAutoCtx(false)
		param := &RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "m", Retry: common.GetPointer(0)}

		ch1, g1, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.Equal(t, 1, ch1.Id)
		require.Equal(t, "ga", g1)

		param.IncreaseRetry()
		ch2, g2, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.Equal(t, 1, ch2.Id, "未开启跨分组重试时不应失败转移到下一个 auto 分组")
		require.Equal(t, "ga", g2)
	})

	t.Run("auto with cross_group_retry fails over to next auto group", func(t *testing.T) {
		c := newAutoCtx(true)
		param := &RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "m", Retry: common.GetPointer(0)}

		ch1, g1, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.Equal(t, 1, ch1.Id)
		require.Equal(t, "ga", g1)

		param.IncreaseRetry()
		ch2, g2, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.Equal(t, 2, ch2.Id, "开启跨分组重试时应失败转移到下一个 auto 分组")
		require.Equal(t, "gb", g2)
	})
}

func TestResolveTokenPriorityGroups(t *testing.T) {
	origAuto := setting.AutoGroups2JsonString()
	origUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(origAuto)
		_ = setting.UpdateUserUsableGroupsByJSONString(origUsable)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"d","vip":"v","svip":"s"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["svip","vip"]`))

	t.Run("concrete groups preserve priority order", func(t *testing.T) {
		got := ResolveTokenPriorityGroups([]string{"vip", "default", "svip"}, "default")
		require.Equal(t, []string{"vip", "default", "svip"}, got)
	})
	t.Run("dedup keeps first occurrence", func(t *testing.T) {
		got := ResolveTokenPriorityGroups([]string{"vip", "vip", "default"}, "default")
		require.Equal(t, []string{"vip", "default"}, got)
	})
	t.Run("auto expands in place and dedups", func(t *testing.T) {
		// auto → [svip, vip]（按 autoGroups 顺序、过滤用户可用分组）
		got := ResolveTokenPriorityGroups([]string{"default", "auto"}, "default")
		require.Equal(t, []string{"default", "svip", "vip"}, got)
	})
	t.Run("empty input yields empty", func(t *testing.T) {
		require.Empty(t, ResolveTokenPriorityGroups(nil, "default"))
	})
}
