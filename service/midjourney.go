package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

func CovertMjpActionToModelName(mjAction string) string {
	modelName := "mj_" + strings.ToLower(mjAction)
	if mjAction == constant.MjActionSwapFace {
		modelName = "swap_face"
	}
	return modelName
}

// PrepareMidjourneyTaskBilling sets the durable refund marker before the task is inserted.
func PrepareMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, quota int, shouldBill bool) (bool, error) {
	if task == nil {
		return false, errors.New("Midjourney task is nil")
	}
	task.Quota = 0
	task.TokenId = 0
	task.BillingChannelId = 0
	if !shouldBill {
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if quota < 0 || quota > common.MaxQuota {
		return false, fmt.Errorf("quota out of range: %d", quota)
	}
	if relayInfo.BillingSource == BillingSourceSubscription {
		return false, errors.New("legacy Midjourney billing does not support subscriptions")
	}

	task.Quota = quota
	task.BillingChannelId = task.ChannelId
	if relayInfo.ChannelMeta != nil && relayInfo.ChannelId > 0 {
		task.BillingChannelId = relayInfo.ChannelId
	}
	return true, nil
}

// SettleMidjourneyTaskBilling charges a persisted legacy task and records the applied stages.
func SettleMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, prepared bool) (bool, error) {
	if !prepared {
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if task == nil || task.Id == 0 {
		return false, errors.New("Midjourney task must be persisted before billing")
	}

	quota := task.Quota
	baseStage := model.TaskBillingStageParams{
		TaskType:          model.TaskBillingTypeMidjourney,
		TaskRecordId:      int64(task.Id),
		Operation:         "submit",
		Delta:             quota,
		TargetQuota:       quota,
		RequestCountDelta: 1,
		UserId:            task.UserId,
		TokenId:           relayInfo.TokenId,
		TokenKey:          relayInfo.TokenKey,
		ChannelId:         task.GetBillingChannelId(),
		BillingSource:     BillingSourceWallet,
	}
	fundingStage := baseStage
	fundingStage.Stage = model.TaskBillingStageFunding
	if _, err := model.ApplyTaskBillingStage(fundingStage); err != nil {
		if errors.Is(err, model.ErrTaskBillingCommitUncertain) ||
			errors.Is(err, model.ErrTaskBillingStageConflict) ||
			errors.Is(err, model.ErrTaskBillingOperationPending) ||
			!errors.Is(err, model.ErrTaskBillingStageNotCommitted) {
			// A conflict, pending operation, or unknown COMMIT outcome can have a
			// durable charge. Retain the marker for an idempotent retry/reconciliation.
			return false, err
		}
		task.Quota = 0
		task.TokenId = 0
		task.BillingChannelId = 0
		if updateErr := task.UpdateBillingState(); updateErr != nil {
			return false, errors.Join(err, fmt.Errorf("clear Midjourney billing state: %w", updateErr))
		}
		return false, err
	}

	if !relayInfo.IsPlayground {
		tokenStage := baseStage
		tokenStage.Stage = model.TaskBillingStageToken
		if _, err := model.ApplyTaskBillingStage(tokenStage); err != nil {
			if errors.Is(err, model.ErrTaskBillingCommitUncertain) {
				// A single missing journal read cannot prove that the token charge
				// rolled back. Keep funding applied and let the same submit operation
				// retry/reconcile the token attempt before any reverse compensation.
				return false, err
			}
			rollbackErr := compensateMidjourneySubmit(task, fundingStage, nil)
			return false, errors.Join(err, rollbackErr)
		}
	}

	finalizeStage := baseStage
	finalizeStage.Stage = model.TaskBillingStageFinalize
	finalizeStage.TargetQuota = quota
	if _, err := model.ApplyTaskBillingStage(finalizeStage); err != nil {
		if errors.Is(err, model.ErrTaskBillingCommitUncertain) {
			// Finalize may already be durable. Undoing token/funding against one
			// stale journal snapshot could create a finalized free task.
			return false, err
		}
		var tokenStage *model.TaskBillingStageParams
		if !relayInfo.IsPlayground {
			stage := baseStage
			stage.Stage = model.TaskBillingStageToken
			tokenStage = &stage
		}
		rollbackErr := compensateMidjourneySubmit(task, fundingStage, tokenStage)
		return false, errors.Join(err, rollbackErr)
	}

	task.TokenId = 0
	if !relayInfo.IsPlayground {
		task.TokenId = relayInfo.TokenId
	}
	updateResult := model.DB.Model(&model.Midjourney{}).
		Where("id = ? AND quota = ?", task.Id, quota).
		Updates(map[string]interface{}{
			"token_id":           task.TokenId,
			"billing_channel_id": task.BillingChannelId,
		})
	if updateResult.Error != nil {
		// The ledger, not these convenience columns, is the durable source of
		// truth. The quota predicate also prevents a concurrent refund from being
		// overwritten by this submit completion's stale in-memory marker.
		return true, fmt.Errorf("update Midjourney billing state: %w", updateResult.Error)
	}
	return true, nil
}

func compensateMidjourneySubmit(task *model.Midjourney, fundingStage model.TaskBillingStageParams, tokenStage *model.TaskBillingStageParams) error {
	if tokenStage != nil {
		if _, err := model.UndoTaskBillingStage(*tokenStage); err != nil {
			return fmt.Errorf("compensate Midjourney token stage: %w", err)
		}
	}
	if _, err := model.UndoTaskBillingStage(fundingStage); err != nil {
		return fmt.Errorf("compensate Midjourney funding stage: %w", err)
	}
	originalQuota := task.Quota
	task.Quota = 0
	if err := task.UpdateBillingState(); err != nil {
		task.Quota = originalQuota
		return fmt.Errorf("clear compensated Midjourney billing marker: %w", err)
	}
	return nil
}

// RefundMidjourneyQuota reverses every accounting element recorded for a billed legacy task.
func RefundMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) bool {
	quota := task.Quota
	if quota < 0 {
		logger.LogWarn(ctx, fmt.Sprintf("cannot refund Midjourney task %s with negative quota %d", task.MjId, quota))
		return false
	}
	if quota == 0 {
		return true
	}
	billingChannelId := task.GetBillingChannelId()
	fundingQuota, tokenQuota, usageQuota, tokenId, err := midjourneyAppliedBilling(task)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("读取 Midjourney 账本失败 task %s: %s", task.MjId, err.Error()))
		return false
	}
	baseStage := model.TaskBillingStageParams{
		TaskType: model.TaskBillingTypeMidjourney, TaskRecordId: int64(task.Id), Operation: "refund",
		UserId: task.UserId, TokenId: tokenId, ChannelId: billingChannelId, BillingSource: BillingSourceWallet,
		TargetQuota: 0,
	}
	if fundingQuota == 0 && tokenQuota == 0 && usageQuota == 0 {
		task.Quota = 0
		if err := task.UpdateBillingState(); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("清除已补偿 Midjourney 账单标记失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		return true
	}
	if fundingQuota != 0 {
		fundingStage := baseStage
		fundingStage.Stage = model.TaskBillingStageFunding
		fundingStage.Delta = -fundingQuota
		if _, err := model.ApplyTaskBillingStage(fundingStage); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 用户额度失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}
	if tokenQuota != 0 {
		tokenKey := resolveTokenKey(ctx, tokenId, task.MjId)
		if tokenKey == "" {
			return false
		}
		tokenStage := baseStage
		tokenStage.Stage = model.TaskBillingStageToken
		tokenStage.Delta = -tokenQuota
		tokenStage.TokenKey = tokenKey
		if _, err := model.ApplyTaskBillingStage(tokenStage); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 令牌额度失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}
	finalizeStage := baseStage
	finalizeStage.Stage = model.TaskBillingStageFinalize
	finalizeStage.Delta = -usageQuota
	finalizeStage.TargetQuota = 0
	finalized, err := model.ApplyTaskBillingStage(finalizeStage)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("完成 Midjourney 退款记账失败 task %s: %s", task.MjId, err.Error()))
		return false
	}
	task.Quota = 0
	if !finalized {
		return true
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: billingChannelId,
		ModelName: CovertMjpActionToModelName(task.Action),
		Quota:     quota,
		TokenId:   tokenId,
		Other: map[string]interface{}{
			"task_id": task.MjId,
			"reason":  reason,
		},
	})

	return true
}

func midjourneyAppliedBilling(task *model.Midjourney) (fundingQuota, tokenQuota, usageQuota, tokenId int, err error) {
	if task.Quota < 0 {
		return 0, 0, 0, 0, fmt.Errorf("Midjourney task quota cannot be negative: %d", task.Quota)
	}
	// Read all submit stages in one statement. If finalize is visible, every
	// preceding stage from that operation is visible in the same snapshot too.
	records, err := model.GetTaskBillingOperationRecords(model.TaskBillingTypeMidjourney, int64(task.Id), "submit")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	var submitFunding, submitToken, submitFinalize *model.TaskBillingLedger
	for i := range records {
		record := &records[i]
		switch record.Stage {
		case model.TaskBillingStageFunding:
			submitFunding = record
		case model.TaskBillingStageToken:
			submitToken = record
		case model.TaskBillingStageFinalize:
			submitFinalize = record
		}
	}

	// Legacy rows have no submit ledger; retain their original marker semantics.
	if submitFunding == nil {
		if task.BillingChannelId > 0 {
			return 0, 0, 0, 0, fmt.Errorf("%w: Midjourney submit settlement has not started", model.ErrTaskBillingOperationPending)
		}
		legacyTokenQuota := 0
		if task.TokenId > 0 {
			legacyTokenQuota = task.Quota
		}
		return task.Quota, legacyTokenQuota, task.Quota, task.TokenId, nil
	}
	if submitFunding.Undone || submitFinalize == nil || submitFinalize.Undone {
		return 0, 0, 0, 0, fmt.Errorf("%w: Midjourney submit settlement is incomplete", model.ErrTaskBillingOperationPending)
	}
	if !submitFunding.Undone {
		fundingQuota = submitFunding.Delta
	}
	tokenId = task.TokenId
	if submitToken != nil {
		tokenId = submitToken.TokenId
		if !submitToken.Undone {
			tokenQuota = submitToken.Delta
		}
	}
	usageQuota = submitFinalize.Delta
	return fundingQuota, tokenQuota, usageQuota, tokenId, nil
}

func GetMjRequestModel(relayMode int, midjRequest *dto.MidjourneyRequest) (string, *dto.MidjourneyResponse, bool) {
	action := ""
	if relayMode == relayconstant.RelayModeMidjourneyAction {
		// plus request
		err := CoverPlusActionToNormalAction(midjRequest)
		if err != nil {
			return "", err, false
		}
		action = midjRequest.Action
	} else {
		switch relayMode {
		case relayconstant.RelayModeMidjourneyImagine:
			action = constant.MjActionImagine
		case relayconstant.RelayModeMidjourneyVideo:
			action = constant.MjActionVideo
		case relayconstant.RelayModeMidjourneyEdits:
			action = constant.MjActionEdits
		case relayconstant.RelayModeMidjourneyDescribe:
			action = constant.MjActionDescribe
		case relayconstant.RelayModeMidjourneyBlend:
			action = constant.MjActionBlend
		case relayconstant.RelayModeMidjourneyShorten:
			action = constant.MjActionShorten
		case relayconstant.RelayModeMidjourneyChange:
			action = midjRequest.Action
		case relayconstant.RelayModeMidjourneyModal:
			action = constant.MjActionModal
		case relayconstant.RelayModeSwapFace:
			action = constant.MjActionSwapFace
		case relayconstant.RelayModeMidjourneyUpload:
			action = constant.MjActionUpload
		case relayconstant.RelayModeMidjourneySimpleChange:
			params := ConvertSimpleChangeParams(midjRequest.Content)
			if params == nil {
				return "", MidjourneyErrorWrapper(constant.MjRequestError, "invalid_request"), false
			}
			action = params.Action
		case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition, relayconstant.RelayModeMidjourneyNotify:
			return "", nil, true
		default:
			return "", MidjourneyErrorWrapper(constant.MjRequestError, "unknown_relay_action"), false
		}
	}
	modelName := CovertMjpActionToModelName(action)
	return modelName, nil, true
}

func CoverPlusActionToNormalAction(midjRequest *dto.MidjourneyRequest) *dto.MidjourneyResponse {
	// "customId": "MJ::JOB::upsample::2::3dbbd469-36af-4a0f-8f02-df6c579e7011"
	customId := midjRequest.CustomId
	if customId == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "custom_id_is_required")
	}
	splits := strings.Split(customId, "::")
	var action string
	if splits[1] == "JOB" {
		action = splits[2]
	} else {
		action = splits[1]
	}

	if action == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	if strings.Contains(action, "upsample") {
		index, err := strconv.Atoi(splits[3])
		if err != nil {
			return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
		}
		midjRequest.Index = index
		midjRequest.Action = constant.MjActionUpscale
	} else if strings.Contains(action, "variation") {
		midjRequest.Index = 1
		if action == "variation" {
			index, err := strconv.Atoi(splits[3])
			if err != nil {
				return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
			}
			midjRequest.Index = index
			midjRequest.Action = constant.MjActionVariation
		} else if action == "low_variation" {
			midjRequest.Action = constant.MjActionLowVariation
		} else if action == "high_variation" {
			midjRequest.Action = constant.MjActionHighVariation
		}
	} else if strings.Contains(action, "pan") {
		midjRequest.Action = constant.MjActionPan
		midjRequest.Index = 1
	} else if strings.Contains(action, "reroll") {
		midjRequest.Action = constant.MjActionReRoll
		midjRequest.Index = 1
	} else if action == "Outpaint" {
		midjRequest.Action = constant.MjActionZoom
		midjRequest.Index = 1
	} else if action == "CustomZoom" {
		midjRequest.Action = constant.MjActionCustomZoom
		midjRequest.Index = 1
	} else if action == "Inpaint" {
		midjRequest.Action = constant.MjActionInPaint
		midjRequest.Index = 1
	} else {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action:"+customId)
	}
	return nil
}

func ConvertSimpleChangeParams(content string) *dto.MidjourneyRequest {
	split := strings.Split(content, " ")
	if len(split) != 2 {
		return nil
	}

	action := strings.ToLower(split[1])
	changeParams := &dto.MidjourneyRequest{}
	changeParams.TaskId = split[0]

	if action[0] == 'u' {
		changeParams.Action = "UPSCALE"
	} else if action[0] == 'v' {
		changeParams.Action = "VARIATION"
	} else if action == "r" {
		changeParams.Action = "REROLL"
		return changeParams
	} else {
		return nil
	}

	index, err := strconv.Atoi(action[1:2])
	if err != nil || index < 1 || index > 4 {
		return nil
	}
	changeParams.Index = index
	return changeParams
}

func DoMidjourneyHttpRequest(c *gin.Context, timeout time.Duration, fullRequestURL string) (*dto.MidjourneyResponseWithStatusCode, []byte, error) {
	var nullBytes []byte
	//var requestBody io.Reader
	//requestBody = c.Request.Body
	// read request body to json, delete accountFilter and notifyHook
	var mapResult map[string]interface{}
	// if get request, no need to read request body
	if c.Request.Method != "GET" {
		err := json.NewDecoder(c.Request.Body).Decode(&mapResult)
		if err != nil {
			return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_request_body_failed", http.StatusInternalServerError), nullBytes, err
		}
		if !setting.MjAccountFilterEnabled {
			delete(mapResult, "accountFilter")
		}
		if !setting.MjNotifyEnabled {
			delete(mapResult, "notifyHook")
		}
		//req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
		// make new request with mapResult
	}
	if setting.MjModeClearEnabled {
		if prompt, ok := mapResult["prompt"].(string); ok {
			prompt = strings.Replace(prompt, "--fast", "", -1)
			prompt = strings.Replace(prompt, "--relax", "", -1)
			prompt = strings.Replace(prompt, "--turbo", "", -1)

			mapResult["prompt"] = prompt
		}
	}
	reqBody, err := json.Marshal(mapResult)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "marshal_request_body_failed", http.StatusInternalServerError), nullBytes, err
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "create_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	// 使用带有超时的 context 创建新的请求
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	auth := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if auth != "" {
		auth = strings.TrimPrefix(auth, "Bearer ")
		req.Header.Set("mj-api-secret", auth)
	}
	defer cancel()
	resp, err := GetHttpClient().Do(req)
	if err != nil {
		common.SysLog("do request failed: " + err.Error())
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "do_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	statusCode := resp.StatusCode
	//if statusCode != 200  {
	//	return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "bad_response_status_code", statusCode), nullBytes, nil
	//}
	err = req.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	err = c.Request.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	var midjResponse dto.MidjourneyResponse
	var midjourneyUploadsResponse dto.MidjourneyUploadResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_response_body_failed", statusCode), nullBytes, err
	}
	CloseResponseBodyGracefully(resp)
	logger.LogDebug(c, "midjourney response body: %s", responseBody)
	if len(responseBody) == 0 {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "empty_response_body", statusCode), responseBody, nil
	} else {
		err = json.Unmarshal(responseBody, &midjResponse)
		if err != nil {
			err2 := json.Unmarshal(responseBody, &midjourneyUploadsResponse)
			if err2 != nil {
				return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "unmarshal_response_body_failed", statusCode), responseBody, err
			}
		}
	}
	//for k, v := range resp.Header {
	//	c.Writer.Header().Set(k, v[0])
	//}
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   midjResponse,
	}, responseBody, nil
}
