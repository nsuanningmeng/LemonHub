package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func taskSubmissionBillingSnapshot(info *relaycommon.RelayInfo, targetQuota int, failure string) model.TaskSubmissionBilling {
	if info != nil {
		if session, ok := info.Billing.(*BillingSession); ok {
			return session.taskSubmissionBillingSnapshot(targetQuota, failure)
		}
	}
	preConsumed := 0
	if info != nil && info.Billing != nil {
		preConsumed = info.Billing.GetPreConsumedQuota()
	} else if info != nil {
		preConsumed = info.FinalPreConsumedQuota
	}
	tokenQuota := 0
	if info != nil && !info.IsPlayground && info.TokenId > 0 {
		tokenQuota = preConsumed
	}
	return model.TaskSubmissionBilling{
		PreConsumedQuota: preConsumed,
		TargetQuota:      targetQuota,
		FundingQuota:     preConsumed,
		TokenQuota:       tokenQuota,
		Failure:          failure,
		UpdatedAt:        common.GetTimestamp(),
	}
}

// PersistAndSettleTaskSubmission keeps a newly accepted upstream task hidden
// behind a durable pending marker until settlement, aggregate accounting, and
// the ready transition have all committed. A crashed or ambiguous submission
// remains fail-closed for manual reconciliation instead of entering polling.
func PersistAndSettleTaskSubmission(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task) error {
	if task == nil {
		return errors.New("task is nil")
	}
	if info == nil {
		return errors.New("relay info is nil")
	}
	tokenCharged := !info.IsPlayground && info.TokenId > 0
	task.TokenCharged = common.GetPointer(tokenCharged)
	task.BillingStatus = model.TaskBillingStatusPending
	initialSnapshot := taskSubmissionBillingSnapshot(info, task.Quota, "")
	// Until Settle returns and its exact stage outcomes are persisted, a hard
	// process crash cannot distinguish applied from unapplied funding/token
	// mutations. Persist an honest unknown snapshot from the first INSERT.
	initialSnapshot.FundingUncertain = true
	initialSnapshot.TokenUncertain = tokenCharged
	task.PrivateData.SubmissionBilling = &initialSnapshot
	if err := task.Insert(); err != nil {
		return fmt.Errorf("persist submitted task before billing settlement: %w", err)
	}

	if err := SettleBilling(c, info, task.Quota); err != nil {
		settleErr := fmt.Errorf("settle submitted task billing: %w", err)
		failedSnapshot := taskSubmissionBillingSnapshot(info, task.Quota, "settlement_failed")
		task.PrivateData.SubmissionBilling = &failedSnapshot
		// Refund on the request defer is intentionally suppressed. Its wallet path
		// is not retry-idempotent and cannot safely reconcile a partial/ambiguous
		// settlement from a single target quota. The durable snapshot is the manual
		// recovery source of truth.
		info.Billing = nil
		_, reviewErr := task.MarkBillingManualReview("settlement_failed")
		return errors.Join(settleErr, reviewErr)
	}

	settledSnapshot := taskSubmissionBillingSnapshot(info, task.Quota, "")
	task.PrivateData.SubmissionBilling = &settledSnapshot
	tokenId := 0
	if tokenCharged {
		tokenId = task.PrivateData.TokenId
	}
	baseline := model.TaskBillingStageParams{
		TaskType:          model.TaskBillingTypeTask,
		TaskRecordId:      task.ID,
		Operation:         "submit",
		Stage:             model.TaskBillingStageAggregateBaseline,
		Delta:             task.Quota,
		TargetQuota:       task.Quota,
		RequestCountDelta: 1,
		UserId:            task.UserId,
		TokenId:           tokenId,
		ChannelId:         task.ChannelId,
		SubscriptionId:    task.PrivateData.SubscriptionId,
		BillingSource:     task.PrivateData.BillingSource,
		SubmissionBilling: &settledSnapshot,
	}
	if _, err := model.ApplyTaskBillingStage(baseline); err != nil {
		baselineErr := fmt.Errorf("record task aggregate baseline: %w", err)
		failedSnapshot := settledSnapshot
		failedSnapshot.Failure = "aggregate_baseline_failed"
		failedSnapshot.UpdatedAt = common.GetTimestamp()
		task.PrivateData.SubmissionBilling = &failedSnapshot
		// Settlement has already reached a durable or ambiguous outcome. The
		// request-level defer must not run a second, non-idempotent refund while
		// the persisted recovery snapshot is awaiting reconciliation.
		info.Billing = nil
		_, reviewErr := task.MarkBillingManualReview("aggregate_baseline_failed")
		return errors.Join(baselineErr, reviewErr)
	}
	task.PrivateData.AggregateUsageState = model.TaskAggregateUsageAccounted
	task.BillingStatus = model.TaskBillingStatusReady
	return nil
}

func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task) error {
	if task == nil || task.ID <= 0 || !task.BillingReady() {
		return errors.New("task billing must be ready before recording consumption")
	}
	if task.BillingStatus == "" && task.PrivateData.AggregateUsageState != model.TaskAggregateUsageAccounted {
		tokenId := 0
		if task.TokenBillingEnabled() {
			tokenId = task.PrivateData.TokenId
		}
		baseline := model.TaskBillingStageParams{
			TaskType:          model.TaskBillingTypeTask,
			TaskRecordId:      task.ID,
			Operation:         "submit",
			Stage:             model.TaskBillingStageAggregateBaseline,
			Delta:             task.Quota,
			TargetQuota:       task.Quota,
			RequestCountDelta: 1,
			UserId:            task.UserId,
			TokenId:           tokenId,
			ChannelId:         task.ChannelId,
			SubscriptionId:    task.PrivateData.SubscriptionId,
			BillingSource:     task.PrivateData.BillingSource,
		}
		if _, err := model.ApplyTaskBillingStage(baseline); err != nil {
			return fmt.Errorf("record legacy task aggregate baseline: %w", err)
		}
		task.PrivateData.AggregateUsageState = model.TaskAggregateUsageAccounted
		task.BillingStatus = model.TaskBillingStatusReady
	}

	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	return nil
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

// taskAdjustFunding 调整任务的资金来源（钱包或订阅），delta > 0 表示扣费，delta < 0 表示退还。
func taskAdjustFunding(task *model.Task, delta int) error {
	if taskIsSubscription(task) {
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta, false)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

func taskBillingOperationSnapshot(
	task *model.Task,
	baseStage model.TaskBillingStageParams,
	preQuota int,
	failure string,
	fundingMissingState string,
	tokenMissingState string,
) model.TaskBillingOperation {
	if fundingMissingState == "" {
		fundingMissingState = model.TaskBillingStageStatePending
	}
	if tokenMissingState == "" {
		if task.TokenBillingEnabled() {
			tokenMissingState = model.TaskBillingStageStatePending
		} else {
			tokenMissingState = model.TaskBillingStageStateNotApplicable
		}
	}
	snapshot := model.TaskBillingOperation{
		Operation: baseStage.Operation, PreQuota: preQuota, TargetQuota: baseStage.TargetQuota,
		Delta: baseStage.Delta, FundingStage: fundingMissingState, TokenStage: tokenMissingState,
		FinalizeStage: model.TaskBillingStageStatePending, Failure: failure,
	}
	ledgers, err := model.GetTaskBillingOperationRecords(baseStage.TaskType, baseStage.TaskRecordId, baseStage.Operation)
	if err != nil {
		snapshot.Failure = strings.TrimSpace(snapshot.Failure + "; journal_read_failed")
		return snapshot
	}
	for i := range ledgers {
		state := model.TaskBillingStageStateApplied
		if ledgers[i].Undone {
			state = model.TaskBillingStageStateUndone
		}
		switch ledgers[i].Stage {
		case model.TaskBillingStageFunding:
			snapshot.FundingStage = state
		case model.TaskBillingStageToken:
			snapshot.TokenStage = state
		case model.TaskBillingStageFinalize:
			snapshot.FinalizeStage = state
		}
	}
	return snapshot
}

func markTaskBillingOperationManual(
	ctx context.Context,
	task *model.Task,
	baseStage model.TaskBillingStageParams,
	preQuota int,
	failure string,
	fundingMissingState string,
	tokenMissingState string,
) {
	snapshot := taskBillingOperationSnapshot(
		task, baseStage, preQuota, failure, fundingMissingState, tokenMissingState,
	)
	if err := task.UpdateBillingReconciliationSnapshot(snapshot, true); err != nil {
		logger.LogError(ctx, fmt.Sprintf("persist task billing manual-review snapshot %s: %s", task.TaskID, err.Error()))
		return
	}
	if err := model.CompleteTaskBillingOperationFences(baseStage); err != nil {
		// Expected for a genuinely partial/uncertain operation. The persistent
		// fence remains the spendability guard until an operator completes the
		// journal compensation or finalize stage.
		logger.LogError(ctx, fmt.Sprintf("task billing operation remains fenced for manual review %s: %s", task.TaskID, err.Error()))
	}
}

func markTaskTokenStageFailureManual(
	ctx context.Context,
	task *model.Task,
	baseStage model.TaskBillingStageParams,
	applyErr error,
) {
	tokenMissingState := model.TaskBillingStageStateNotApplied
	if errors.Is(applyErr, model.ErrTaskBillingCommitUncertain) {
		tokenMissingState = model.TaskBillingStageStatePending
	}
	failure := "settle_token_or_compensation_failed"
	if baseStage.Operation == "refund" {
		failure = "refund_token_or_compensation_failed"
	}
	markTaskBillingOperationManual(ctx, task, baseStage, task.Quota, failure,
		model.TaskBillingStageStatePending, tokenMissingState)
}

// compensateTaskTokenStageFailure unwinds token before funding. When the
// token COMMIT is uncertain, a missing row from one immediate read is not
// proof of rollback; in that case the applied funding stage remains durable so
// a later retry can finish the same operation without creating a free credit.
func compensateTaskTokenStageFailure(fundingStage, tokenStage model.TaskBillingStageParams, applyErr error) error {
	if errors.Is(applyErr, model.ErrTaskBillingCommitUncertain) {
		record, err := model.GetTaskBillingStageRecord(
			tokenStage.TaskType, tokenStage.TaskRecordId, tokenStage.Operation, tokenStage.Stage,
		)
		if err != nil {
			return fmt.Errorf("recheck uncertain token stage before compensation: %w", err)
		}
		if record == nil {
			return model.ErrTaskBillingCommitUncertain
		}
	}
	if _, err := model.UndoTaskBillingStage(tokenStage); err != nil {
		return fmt.Errorf("compensate token stage: %w", err)
	}
	if _, err := model.UndoTaskBillingStage(fundingStage); err != nil {
		return fmt.Errorf("compensate funding stage: %w", err)
	}
	return nil
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，退还资金与令牌额度，并回减用户和渠道用量。
// 返回资金来源是否已成功退还；失败时保留 quota，供显式重试或人工对账。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	if task == nil {
		logger.LogError(ctx, "cannot refund a nil task")
		return false
	}
	if task.ID <= 0 {
		if task.Quota == 0 {
			return true
		}
		logger.LogError(ctx, fmt.Sprintf("cannot refund an unpersisted task %s", task.TaskID))
		return false
	}
	var persisted model.Task
	if err := model.DB.Where("id = ?", task.ID).First(&persisted).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("cannot read task billing marker %s: %s", task.TaskID, err.Error()))
		return false
	}
	*task = persisted
	if !task.BillingReady() {
		logger.LogError(ctx, fmt.Sprintf("cannot automatically refund task %s while billing status is %s", task.TaskID, task.BillingStatus))
		return false
	}
	quota := persisted.Quota
	if quota < 0 {
		logger.LogError(ctx, fmt.Sprintf("cannot refund task %s with negative persisted quota %d", task.TaskID, quota))
		return false
	}
	if quota == 0 {
		return true
	}

	tokenId := 0
	if task.TokenBillingEnabled() {
		tokenId = task.PrivateData.TokenId
	}
	baseStage := model.TaskBillingStageParams{
		TaskType:       model.TaskBillingTypeTask,
		TaskRecordId:   task.ID,
		Operation:      "refund",
		Delta:          -quota,
		UserId:         task.UserId,
		TokenId:        tokenId,
		ChannelId:      task.ChannelId,
		SubscriptionId: task.PrivateData.SubscriptionId,
		BillingSource:  task.PrivateData.BillingSource,
		TargetQuota:    0,
	}
	initialOperation := taskBillingOperationSnapshot(task, baseStage, quota, "", "", "")
	won, beginErr := task.BeginBillingReconciliation(initialOperation)
	if beginErr != nil || !won {
		logger.LogError(ctx, fmt.Sprintf("cannot begin task refund reconciliation %s: won=%t error=%v", task.TaskID, won, beginErr))
		return false
	}
	tokenKey := ""
	if task.TokenBillingEnabled() {
		tokenKey = resolveTokenKey(ctx, tokenId, task.TaskID)
		if tokenKey == "" {
			markTaskBillingOperationManual(ctx, task, baseStage, quota, "refund_token_key_unavailable",
				model.TaskBillingStageStateNotApplied, model.TaskBillingStageStateNotApplied)
			return false
		}
		baseStage.TokenKey = tokenKey
	}
	operationFence, fenceErr := model.BeginTaskBillingOperationFences(baseStage)
	baseStage.OperationFence = operationFence
	if fenceErr != nil {
		markTaskBillingOperationManual(ctx, task, baseStage, quota, "refund_fence_failed",
			model.TaskBillingStageStateNotApplied, model.TaskBillingStageStateNotApplied)
		return false
	}

	// Each mutation and its stage marker commit together. A retry skips only the
	// stages that are known to have committed.
	fundingStage := baseStage
	fundingStage.Stage = model.TaskBillingStageFunding
	if _, err := model.ApplyTaskBillingStage(fundingStage); err != nil {
		fundingMissingState := model.TaskBillingStageStatePending
		if !errors.Is(err, model.ErrTaskBillingCommitUncertain) {
			fundingMissingState = model.TaskBillingStageStateNotApplied
		}
		markTaskBillingOperationManual(ctx, task, baseStage, quota, "refund_funding_failed",
			fundingMissingState, model.TaskBillingStageStateNotApplied)
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}

	if task.TokenBillingEnabled() {
		tokenStage := baseStage
		tokenStage.Stage = model.TaskBillingStageToken
		tokenStage.TokenKey = tokenKey
		if _, err := model.ApplyTaskBillingStage(tokenStage); err != nil {
			rollbackErr := compensateTaskTokenStageFailure(fundingStage, tokenStage, err)
			markTaskTokenStageFailureManual(ctx, task, baseStage, err)
			logger.LogWarn(ctx, fmt.Sprintf("退还令牌额度失败 task %s: %s", task.TaskID, err.Error()))
			if rollbackErr != nil {
				logger.LogError(ctx, fmt.Sprintf("补偿已完成的资金退款阶段失败 task %s: %s", task.TaskID, rollbackErr.Error()))
			}
			return false
		}
	}

	finalizeStage := baseStage
	finalizeStage.Stage = model.TaskBillingStageFinalize
	finalizeStage.TargetQuota = 0
	finalized, err := model.ApplyTaskBillingStage(finalizeStage)
	if err != nil {
		markTaskBillingOperationManual(ctx, task, baseStage, quota, "refund_finalize_failed",
			model.TaskBillingStageStatePending, model.TaskBillingStageStatePending)
		logger.LogWarn(ctx, fmt.Sprintf("完成退款记账失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}
	task.Quota = 0
	completedSnapshot := taskBillingOperationSnapshot(task, baseStage, quota, "", "", "")
	if err := task.UpdateBillingReconciliationSnapshot(completedSnapshot, false); err != nil {
		logger.LogError(ctx, fmt.Sprintf("persist completed task refund snapshot %s: %s", task.TaskID, err.Error()))
		return false
	}
	ready, readyErr := task.CompleteBillingReconciliation()
	if readyErr != nil || !ready {
		logger.LogError(ctx, fmt.Sprintf("complete task refund reconciliation %s: ready=%t error=%v", task.TaskID, ready, readyErr))
		return false
	}
	if fenceErr := model.CompleteTaskBillingOperationFences(baseStage); fenceErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task refund %s completed while another billing operation keeps quota fenced: %s",
			task.TaskID, fenceErr.Error()))
	}
	if !finalized {
		return true
	}

	// The financial transaction is complete before the informational log. A
	// retry therefore cannot duplicate either a refund or its log.
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   tokenId,
		Group:     task.Group,
		Other:     other,
	})

	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 可选：若计算 actualQuota 时发生额度饱和，将其记入日志 admin_info（仅管理员可见）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	if task == nil {
		logger.LogError(ctx, "cannot settle a nil task")
		return
	}
	if actualQuota < 0 || actualQuota > common.MaxQuota {
		logger.LogError(ctx, fmt.Sprintf("cannot settle task %s with out-of-range actual quota %d", task.TaskID, actualQuota))
		return
	}
	if task.ID <= 0 {
		logger.LogError(ctx, fmt.Sprintf("cannot settle an unpersisted task %s", task.TaskID))
		return
	}
	var persisted model.Task
	if err := model.DB.Where("id = ?", task.ID).First(&persisted).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("cannot read task billing marker %s: %s", task.TaskID, err.Error()))
		return
	}
	*task = persisted
	if !task.BillingReady() {
		logger.LogError(ctx, fmt.Sprintf("cannot automatically settle task %s while billing status is %s", task.TaskID, task.BillingStatus))
		return
	}
	preConsumedQuota := persisted.Quota
	if preConsumedQuota < 0 || preConsumedQuota > common.MaxQuota {
		logger.LogError(ctx, fmt.Sprintf("cannot settle task %s with out-of-range persisted quota %d", task.TaskID, preConsumedQuota))
		return
	}
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
	} else {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
			task.TaskID,
			logger.LogQuota(quotaDelta),
			logger.LogQuota(actualQuota),
			logger.LogQuota(preConsumedQuota),
			reason,
		))
	}

	operation := fmt.Sprintf("settle:%d", actualQuota)
	tokenId := 0
	if task.TokenBillingEnabled() {
		tokenId = task.PrivateData.TokenId
	}
	baseStage := model.TaskBillingStageParams{
		TaskType:       model.TaskBillingTypeTask,
		TaskRecordId:   task.ID,
		Operation:      operation,
		Delta:          quotaDelta,
		UserId:         task.UserId,
		TokenId:        tokenId,
		ChannelId:      task.ChannelId,
		SubscriptionId: task.PrivateData.SubscriptionId,
		BillingSource:  task.PrivateData.BillingSource,
		TargetQuota:    actualQuota,
	}
	initialOperation := taskBillingOperationSnapshot(task, baseStage, preConsumedQuota, "", "", "")
	won, beginErr := task.BeginBillingReconciliation(initialOperation)
	if beginErr != nil || !won {
		logger.LogError(ctx, fmt.Sprintf("cannot begin task settlement reconciliation %s: won=%t error=%v", task.TaskID, won, beginErr))
		return
	}
	tokenKey := ""
	if task.TokenBillingEnabled() {
		tokenKey = resolveTokenKey(ctx, tokenId, task.TaskID)
		if tokenKey == "" {
			markTaskBillingOperationManual(ctx, task, baseStage, preConsumedQuota, "settle_token_key_unavailable",
				model.TaskBillingStageStateNotApplied, model.TaskBillingStageStateNotApplied)
			return
		}
		baseStage.TokenKey = tokenKey
	}
	operationFence, fenceErr := model.BeginTaskBillingOperationFences(baseStage)
	baseStage.OperationFence = operationFence
	if fenceErr != nil {
		markTaskBillingOperationManual(ctx, task, baseStage, preConsumedQuota, "settle_fence_failed",
			model.TaskBillingStageStateNotApplied, model.TaskBillingStageStateNotApplied)
		return
	}
	reconciliationComplete := false
	defer func() {
		if !reconciliationComplete && task.BillingStatus == model.TaskBillingStatusReconciling {
			markTaskBillingOperationManual(ctx, task, baseStage, preConsumedQuota, "settlement_incomplete",
				model.TaskBillingStageStatePending, model.TaskBillingStageStatePending)
		}
	}()
	fundingStage := baseStage
	fundingStage.Stage = model.TaskBillingStageFunding
	if _, err := model.ApplyTaskBillingStage(fundingStage); err != nil {
		fundingMissingState := model.TaskBillingStageStatePending
		if !errors.Is(err, model.ErrTaskBillingCommitUncertain) {
			fundingMissingState = model.TaskBillingStageStateNotApplied
		}
		markTaskBillingOperationManual(ctx, task, baseStage, preConsumedQuota, "settle_funding_failed",
			fundingMissingState, model.TaskBillingStageStateNotApplied)
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	if task.TokenBillingEnabled() {
		tokenStage := baseStage
		tokenStage.Stage = model.TaskBillingStageToken
		tokenStage.TokenKey = tokenKey
		if _, err := model.ApplyTaskBillingStage(tokenStage); err != nil {
			rollbackErr := compensateTaskTokenStageFailure(fundingStage, tokenStage, err)
			markTaskTokenStageFailureManual(ctx, task, baseStage, err)
			logger.LogError(ctx, fmt.Sprintf("差额结算令牌调整失败 task %s: %s", task.TaskID, err.Error()))
			if rollbackErr != nil {
				logger.LogError(ctx, fmt.Sprintf("补偿差额结算资金阶段失败 task %s: %s", task.TaskID, rollbackErr.Error()))
			}
			return
		}
	}

	finalizeStage := baseStage
	finalizeStage.Stage = model.TaskBillingStageFinalize
	finalizeStage.TargetQuota = actualQuota
	finalized, err := model.ApplyTaskBillingStage(finalizeStage)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
		return
	}
	task.Quota = actualQuota
	completedSnapshot := taskBillingOperationSnapshot(task, baseStage, preConsumedQuota, "", "", "")
	if err := task.UpdateBillingReconciliationSnapshot(completedSnapshot, false); err != nil {
		logger.LogError(ctx, fmt.Sprintf("persist completed task settlement snapshot %s: %s", task.TaskID, err.Error()))
		return
	}
	ready, readyErr := task.CompleteBillingReconciliation()
	if readyErr != nil || !ready {
		logger.LogError(ctx, fmt.Sprintf("complete task settlement reconciliation %s: ready=%t error=%v", task.TaskID, ready, readyErr))
		return
	}
	reconciliationComplete = true
	if fenceErr := model.CompleteTaskBillingOperationFences(baseStage); fenceErr != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task settlement %s completed while another billing operation keeps quota fenced: %s",
			task.TaskID, fenceErr.Error()))
	}
	if !finalized {
		return
	}
	if quotaDelta == 0 {
		return
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   tokenId,
		Group:     task.Group,
		Other:     other,
		NodeName:  task.PrivateData.NodeName,
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	var modelRatio, finalGroupRatio float64
	if billingContext := task.PrivateData.BillingContext; billingContext != nil {
		// New tasks persist the exact submission-time pricing context. Reuse it so
		// polling cannot reprice a task after a configuration change, and so the
		// selected group's special ratio survives multi-group failover.
		modelRatio = billingContext.ModelRatio
		finalGroupRatio = billingContext.GroupRatio
		if modelRatio <= 0 {
			return
		}
	} else {
		// Legacy tasks have no billing snapshot and must fall back to current
		// settings. Keep the user's group distinct from the actual routing group
		// when resolving a group-specific override.
		modelName := taskModelName(task)
		var hasRatioSetting bool
		modelRatio, hasRatioSetting, _ = ratio_setting.GetModelRatio(modelName)
		if !hasRatioSetting || modelRatio <= 0 {
			return
		}

		usingGroup := task.Group
		userGroup := ""
		if user, err := model.GetUserById(task.UserId, false); err == nil {
			userGroup = user.Group
		}
		if usingGroup == "" {
			usingGroup = userGroup
		}
		if usingGroup == "" {
			return
		}

		finalGroupRatio = ratio_setting.GetGroupRatio(usingGroup)
		if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup); ok {
			finalGroupRatio = userGroupRatio
		}
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(task.PrivateData.BillingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}

	// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio * otherMultiplier（饱和转换，防止溢出成负数）
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * finalGroupRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, finalGroupRatio, otherMultiplier)
	RecalculateTaskQuota(ctx, task, actualQuota, reason, clamp)
}
