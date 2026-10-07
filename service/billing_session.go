package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	language         string // 请求语言快照；Reserve 无需保留请求上下文
	funding          FundingSource
	preConsumedQuota int  // 实际预扣额度（信任用户可能为 0）
	fundingConsumed  int  // 资金来源当前已扣额度（用于任务提交恢复快照）
	tokenConsumed    int  // 令牌额度实际扣减量
	extraReserved    int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted          bool // 是否命中信任额度旁路
	fundingSettled   bool // funding.Settle 已成功，资金来源已提交
	settled          bool // Settle 全部完成（资金 + 令牌）
	settledQuota     int  // 订阅最终receipt的实际额度，防止冲突目标重入
	refunded         bool // Refund 已调用
	fundingUncertain bool // 资金变更返回错误，持久结果需人工核对
	tokenUncertain   bool // token 变更返回错误，持久结果需人工核对
	mu               sync.Mutex
}

type billingQuotaDiagnostic struct {
	message string
	cause   error
}

func (e *billingQuotaDiagnostic) Error() string { return e.message }
func (e *billingQuotaDiagnostic) Unwrap() error { return e.cause }

func localizedBillingQuotaError(language, key, legacy string, values map[string]any, cause error) error {
	message := i18n.Translate(language, key, values)
	if message != legacy {
		// Older downstream instances use this exact Chinese diagnostic for
		// keyword matching. Keep it intact alongside the localized explanation.
		message += " (" + legacy + ")"
	}
	return &billingQuotaDiagnostic{message: message, cause: cause}
}

func walletQuotaDiagnostic(language string, quota int, cause error) error {
	remaining := logger.FormatQuota(quota)
	return localizedBillingQuotaError(language, "quota.wallet_insufficient",
		fmt.Sprintf("用户额度不足, 剩余额度: %s", remaining),
		map[string]any{"Remaining": remaining}, cause)
}

func preconsumeQuotaDiagnostic(language string, quota, requiredQuota int) error {
	remaining, required := logger.FormatQuota(quota), logger.FormatQuota(requiredQuota)
	return localizedBillingQuotaError(language, "quota.preconsume_insufficient",
		fmt.Sprintf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", remaining, required),
		map[string]any{"Remaining": remaining, "Required": required}, nil)
}

func subscriptionQuotaDiagnostic(language string, cause error) error {
	reason := cause.Error()
	return localizedBillingQuotaError(language, "quota.subscription_insufficient",
		fmt.Sprintf("订阅额度不足或未配置订阅: %s", reason),
		map[string]any{"Reason": reason}, cause)
}

// Settle 根据实际消耗额度进行结算。
// 资金来源和令牌额度分两步提交：若资金来源已提交但令牌调整失败，
// 会标记 fundingSettled 防止 Refund 对已提交的资金来源执行退款。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		if _, ok := s.funding.(*SubscriptionFunding); ok && actualQuota != s.settledQuota {
			return model.ErrSubscriptionBillingConflict
		}
		return nil
	}
	if actualQuota < 0 || actualQuota > common.MaxQuota {
		return fmt.Errorf("actual quota out of range: %d", actualQuota)
	}
	if _, ok := s.funding.(*SubscriptionFunding); ok {
		receipt, err := model.SettleSubscriptionBilling(s.subscriptionBillingParams(actualQuota))
		if err != nil {
			if errors.Is(err, model.ErrSubscriptionBillingUncertain) {
				s.fundingUncertain = true
				s.tokenUncertain = !s.relayInfo.IsPlayground
			}
			return err
		}
		s.fundingUncertain, s.tokenUncertain = false, false
		s.fundingConsumed = actualQuota
		if !s.relayInfo.IsPlayground {
			s.tokenConsumed = actualQuota
		}
		s.fundingSettled, s.settled, s.settledQuota = true, true, actualQuota
		s.relayInfo.SubscriptionPostDelta = receipt.SubscriptionDelta
		s.relayInfo.SubscriptionWalletQuota = receipt.WalletConsumed
		s.relayInfo.SubscriptionSettlementApplied = true
		s.relayInfo.SubscriptionAmountUsedAfterSettlement = receipt.SubscriptionUsedAfter
		s.relayInfo.SubscriptionAmountTotal = receipt.SubscriptionTotal
		return nil
	}

	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		return nil
	}
	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）
	if !s.fundingSettled {
		if err := s.funding.Settle(delta); err != nil {
			s.fundingUncertain = true
			return err
		}
		s.fundingConsumed += delta
	}
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground {
		if delta > 0 {
			tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
		} else {
			tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
		}
		if tokenErr != nil {
			s.tokenUncertain = true
			// The funding stage committed but the token stage did not. Restore only
			// the stage that actually committed, leaving the original pre-consume
			// state available to Refund.
			rollbackErr := s.funding.Settle(-delta)
			if rollbackErr != nil {
				s.fundingUncertain = true
				s.fundingSettled = true
				return errors.Join(tokenErr, fmt.Errorf("rollback settled funding delta: %w", rollbackErr))
			}
			s.fundingConsumed -= delta
			return tokenErr
		}
	}
	// 3) 更新 relayInfo 上的订阅 PostDelta（用于日志）
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	s.fundingSettled = true
	if !s.relayInfo.IsPlayground {
		s.tokenConsumed += delta
	}
	s.settled = true
	return nil
}

// Refund 退还所有预扣费，幂等安全，异步执行。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	var subscriptionRefund *model.SubscriptionBillingParams
	if _, ok := s.funding.(*SubscriptionFunding); ok {
		if s.fundingUncertain || s.tokenUncertain {
			s.mu.Unlock()
			return
		}
		params := s.subscriptionBillingParams(0)
		subscriptionRefund = &params
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId,
		logger.FormatQuota(s.tokenConsumed),
		s.funding.Source(),
	))

	// 复制需要的值到闭包中
	userId := s.relayInfo.UserId
	tokenId := s.relayInfo.TokenId
	tokenKey := s.relayInfo.TokenKey
	isPlayground := s.relayInfo.IsPlayground
	tokenConsumed := s.tokenConsumed
	extraReserved := s.extraReserved
	subscriptionId := s.relayInfo.SubscriptionId
	funding := s.funding

	billingRefunds.Add(1)
	gopool.Go(func() {
		defer billingRefunds.Done()
		var refundErr error
		defer func() {
			if r := recover(); r != nil {
				recordBillingRefundFailure(errors.Join(refundErr, fmt.Errorf("user %d refund panic: %v", userId, r)))
				panic(r)
			}
			recordBillingRefundFailure(refundErr)
		}()
		if subscriptionRefund != nil {
			if err := model.RefundSubscriptionBilling(*subscriptionRefund); err != nil {
				refundErr = fmt.Errorf("request %s subscription/token refund: %w", subscriptionRefund.RequestId, err)
				common.SysError(refundErr.Error())
			}
			return
		}
		// 1) 退还资金来源
		if err := funding.Refund(); err != nil {
			refundErr = errors.Join(refundErr, fmt.Errorf("user %d funding refund: %w", userId, err))
			common.SysLog("error refunding billing source: " + err.Error())
		}
		if extraReserved > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(subscriptionId, -int64(extraReserved)); err != nil {
				refundErr = errors.Join(refundErr, fmt.Errorf("subscription %d extra reserve refund: %w", subscriptionId, err))
				common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
			}
		}
		// 2) 退还令牌额度
		if tokenConsumed > 0 && !isPlayground {
			if err := model.IncreaseTokenQuota(tokenId, tokenKey, tokenConsumed); err != nil {
				refundErr = errors.Join(refundErr, fmt.Errorf("token %d refund: %w", tokenId, err))
				common.SysLog("error refunding token quota: " + err.Error())
			}
		}
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) taskSubmissionBillingSnapshot(targetQuota int, failure string) model.TaskSubmissionBilling {
	s.mu.Lock()
	defer s.mu.Unlock()
	return model.TaskSubmissionBilling{
		PreConsumedQuota: s.preConsumedQuota,
		TargetQuota:      targetQuota,
		FundingQuota:     s.fundingConsumed,
		TokenQuota:       s.tokenConsumed,
		FundingUncertain: s.fundingUncertain,
		TokenUncertain:   s.tokenUncertain,
		Failure:          failure,
		UpdatedAt:        common.GetTimestamp(),
	}
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.refunded || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if _, ok := s.funding.(*SubscriptionFunding); ok {
		if _, err := model.ReserveSubscriptionBilling(s.subscriptionBillingParams(targetQuota)); err != nil {
			if errors.Is(err, model.ErrSubscriptionBillingUncertain) {
				s.fundingUncertain = true
				s.tokenUncertain = !s.relayInfo.IsPlayground
			}
			if errors.Is(err, model.ErrSubscriptionQuotaInsufficient) {
				return types.NewErrorWithStatusCode(subscriptionQuotaDiagnostic(s.language, err), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			if errors.Is(err, model.ErrSubscriptionTokenQuotaInsufficient) {
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return err
		}
		s.fundingUncertain, s.tokenUncertain = false, false
		s.preConsumedQuota, s.fundingConsumed = targetQuota, targetQuota
		if !s.relayInfo.IsPlayground {
			s.tokenConsumed = targetQuota
		}
		s.extraReserved += delta
		s.syncRelayInfo()
		return nil
	}

	if err := s.reserveFunding(delta); err != nil {
		return err
	}
	if err := s.reserveToken(delta); err != nil {
		s.rollbackFundingReserve(delta)
		return err
	}

	s.preConsumedQuota += delta
	s.fundingConsumed += delta
	if !s.relayInfo.IsPlayground {
		s.tokenConsumed += delta
	}
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 令牌预扣 -> 资金来源预扣。
// 任一步骤失败时原子回滚已完成的步骤。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	if s.language == "" {
		s.language = i18n.GetLangFromContext(c)
	}
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣令牌额度 ----
	if effectiveQuota > 0 && !s.relayInfo.IsPlayground {
		if err := PreConsumeTokenQuota(s.relayInfo, effectiveQuota); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		s.tokenConsumed = effectiveQuota
	}

	// ---- 2) 预扣资金来源 ----
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		// 预扣费失败，回滚令牌额度
		if s.tokenConsumed > 0 && !s.relayInfo.IsPlayground {
			if rollbackErr := model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed); rollbackErr != nil {
				common.SysLog(fmt.Sprintf("error rolling back token quota (userId=%d, tokenId=%d, amount=%d, fundingErr=%s): %s",
					s.relayInfo.UserId, s.relayInfo.TokenId, s.tokenConsumed, err.Error(), rollbackErr.Error()))
			}
			s.tokenConsumed = 0
		}
		// TODO: model 层应定义哨兵错误（如 ErrNoActiveSubscription），用 errors.Is 替代字符串匹配
		if errors.Is(err, ErrInsufficientWalletQuota) {
			userQuota, quotaErr := model.GetUserQuota(s.relayInfo.UserId, false)
			if quotaErr != nil {
				userQuota = 0
			}
			return types.NewErrorWithStatusCode(
				walletQuotaDiagnostic(s.language, userQuota, err),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") {
			return types.NewErrorWithStatusCode(subscriptionQuotaDiagnostic(s.language, err), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	s.preConsumedQuota = effectiveQuota
	s.fundingConsumed = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) subscriptionBillingParams(actualQuota int) model.SubscriptionBillingParams {
	sub := s.funding.(*SubscriptionFunding)
	p := model.SubscriptionBillingParams{RequestId: sub.requestId, UserId: sub.userId, SubscriptionId: sub.subscriptionId,
		PreConsumedQuota: s.preConsumedQuota, ActualQuota: actualQuota,
		AllowWalletOverflow: !s.relayInfo.ForcePreConsume && common.NormalizeBillingPreference(s.relayInfo.UserSetting.BillingPreference) != "subscription_only"}
	if !s.relayInfo.IsPlayground {
		p.TokenId, p.TokenKey, p.TokenConsumedQuota = s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed
	}
	return p
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		// 与结算补扣（SettleBilling 正差额 → WalletFunding.Settle）语义一致：
		// 全额无条件扣减，余额不足的部分记为欠费（余额可为负），不中断请求，
		// 保证日志记录的预扣额度与用户余额的实际变动始终对账一致。
		// DecreaseUserQuota 仅在数据库错误时失败。
		if err := model.DecreaseUserQuota(funding.userId, delta, false); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, int64(delta)); err != nil {
			return types.NewErrorWithStatusCode(
				subscriptionQuotaDiagnostic(s.language, err),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

func (s *BillingSession) rollbackFundingReserve(delta int) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.IncreaseUserQuota(funding.userId, delta, false); err != nil {
			common.SysLog("error rolling back wallet funding reserve: " + err.Error())
		} else {
			funding.consumed -= delta
		}
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, -int64(delta)); err != nil {
			common.SysLog("error rolling back subscription funding reserve: " + err.Error())
		}
	}
}

func (s *BillingSession) reserveToken(delta int) error {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return nil
	}
	if err := PreConsumeTokenQuota(s.relayInfo, delta); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()
	info.SubscriptionWalletQuota = 0
	info.SubscriptionSettlementApplied = false
	info.SubscriptionAmountUsedAfterSettlement = 0

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)
	language := i18n.GetLangFromContext(c)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				walletQuotaDiagnostic(language, userQuota, nil),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				preconsumeQuotaDiagnostic(language, userQuota, preConsumedQuota),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			language:  language,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			language:  language,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				// 仅当用户的活跃订阅允许钱包回退时才回退到钱包，否则返回订阅额度不足错误
				allowOverflow, overflowErr := model.UserActiveSubscriptionsAllowWalletOverflow(relayInfo.UserId)
				if overflowErr != nil {
					return nil, types.NewError(overflowErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
				}
				if allowOverflow {
					return tryWallet()
				}
				return nil, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}
}
