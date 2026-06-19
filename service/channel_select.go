package service

import (
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	RequestPath  string
	Retry        *int
	resetNextTry bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// 支持令牌「多分组优先级失败转移」：令牌的分组解析为有序优先级列表
// （见 ResolveTokenPriorityGroups，"auto" 会就地展开为用户的自动分组）。
//
// For an ordered priority group list (length > 1, or a single "auto" that expands):
// 对于有序优先级分组列表（长度 > 1，或单个会被展开的 "auto"）：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry represents the priority level within current group.
//     priorityRetry 表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// 跨分组失败转移（cross-group failover）门控：
//   - 显式多分组令牌（原始列表长度 > 1）始终启用失败转移；
//   - 单个 "auto" 令牌仍由 token.CrossGroupRetry 标志控制（向后兼容）。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0    Retry=1: GroupA, priority1
//	Retry=2: GroupA exhausted → GroupB, priority0    Retry=3: GroupB, priority1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	rawGroups := getRawTokenGroups(param.Ctx, param.TokenGroup)
	containsAuto := slices.Contains(rawGroups, "auto")

	// 保留原 "auto" 语义：配置了 auto 但全局未启用任何自动分组时直接报错。
	if containsAuto && len(setting.GetAutoGroups()) == 0 {
		return nil, param.TokenGroup, errors.New("auto groups is not enabled")
	}

	priorityGroups := ResolveTokenPriorityGroups(rawGroups, userGroup)

	// 单一具体分组（无 auto 展开）：走原单分组路径，行为完全不变。
	if len(priorityGroups) <= 1 && !containsAuto {
		group := param.TokenGroup
		if len(priorityGroups) == 1 {
			group = priorityGroups[0]
		}
		channel, err := model.GetRandomSatisfiedChannel(group, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, group, err
		}
		return channel, group, nil
	}

	// 多分组 / auto 展开：按优先级列表遍历 + 逐级失败转移。
	// 显式多分组（原始列表 > 1 项）必失败转移；单 "auto" 仍由 token flag 控制。
	explicitMulti := len(rawGroups) > 1
	crossGroupRetry := explicitMulti || common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

	var channel *model.Channel
	selectGroup := param.TokenGroup

	// startGroupIndex: the group index to start searching from
	// startGroupIndex: 开始搜索的分组索引
	startGroupIndex := 0
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}

	for i := startGroupIndex; i < len(priorityGroups); i++ {
		group := priorityGroups[i]
		// Calculate priorityRetry for current group
		// 计算当前分组的 priorityRetry
		priorityRetry := param.GetRetry()
		// If moved to a new group, reset priorityRetry
		// 如果切换到新分组，重置 priorityRetry
		if i > startGroupIndex {
			priorityRetry = 0
		}
		logger.LogDebug(param.Ctx, "Priority selecting group: %s, priorityRetry: %d", group, priorityRetry)

		channel, _ = model.GetRandomSatisfiedChannel(group, param.ModelName, priorityRetry, param.RequestPath)
		if channel == nil {
			// Current group has no available channel for this model, try next group
			// 当前分组没有该模型的可用渠道，尝试下一个分组
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", group, param.ModelName, priorityRetry)
			// 重置状态以尝试下一个分组
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
			// Reset retry counter so outer loop can continue for next group
			// 重置重试计数器，以便外层循环可以为下一个分组继续
			param.SetRetry(0)
			continue
		}
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, group)
		selectGroup = group
		logger.LogDebug(param.Ctx, "Priority selected group: %s", group)

		// Prepare state for next retry
		// 为下一次重试准备状态
		if crossGroupRetry && priorityRetry >= common.RetryTimes {
			// Current group has exhausted all retries, prepare to switch to next group
			// This request still uses current group, but next retry will use next group
			// 当前分组已用完所有重试次数，准备切换到下一个分组
			// 本次请求仍使用当前分组，但下次重试将使用下一个分组
			logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", group, priorityRetry, common.RetryTimes)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			// Reset retry counter so outer loop can continue for next group
			// 重置重试计数器，以便外层循环可以为下一个分组继续
			param.SetRetry(0)
			param.ResetRetryNextTry()
		} else {
			// Stay in current group, save current state
			// 保持在当前分组，保存当前状态
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		}
		break
	}
	return channel, selectGroup, nil
}

// getRawTokenGroups 取令牌的原始有序分组列表：优先读 ContextKeyTokenGroupList，
// 缺失时回退到单值 fallback（兼容 playground / 直接传入 group 的调用）。
func getRawTokenGroups(ctx *gin.Context, fallback string) []string {
	if v, ok := common.GetContextKey(ctx, constant.ContextKeyTokenGroupList); ok {
		if list, ok := v.([]string); ok && len(list) > 0 {
			return list
		}
	}
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}
