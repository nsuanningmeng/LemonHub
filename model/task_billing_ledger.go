package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	TaskBillingTypeTask       = "task"
	TaskBillingTypeMidjourney = "midjourney"

	TaskBillingStageFunding           = "funding"
	TaskBillingStageToken             = "token"
	TaskBillingStageFinalize          = "finalize"
	TaskBillingStageAggregateBaseline = "aggregate_baseline"
)

// TaskBillingLedger is an applied-stage journal.  A row is written in the same
// transaction as its balance mutation, so retries can distinguish an applied
// stage from one that never committed.
type TaskBillingLedger struct {
	Id                int64  `json:"id"`
	TaskType          string `json:"task_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_task_billing_stage,priority:1"`
	TaskRecordId      int64  `json:"task_record_id" gorm:"not null;uniqueIndex:idx_task_billing_stage,priority:2"`
	Operation         string `json:"operation" gorm:"type:varchar(64);not null;uniqueIndex:idx_task_billing_stage,priority:3"`
	Stage             string `json:"stage" gorm:"type:varchar(32);not null;uniqueIndex:idx_task_billing_stage,priority:4"`
	Delta             int    `json:"delta" gorm:"type:int;not null"`
	TargetQuota       int    `json:"target_quota" gorm:"type:int;not null"`
	TokenUsedDelta    int    `json:"token_used_delta" gorm:"type:int;not null"`
	RequestCountDelta int    `json:"request_count_delta" gorm:"type:int;not null"`
	UserId            int    `json:"user_id" gorm:"index"`
	TokenId           int    `json:"token_id" gorm:"index"`
	ChannelId         int    `json:"channel_id" gorm:"index"`
	SubscriptionId    int    `json:"subscription_id" gorm:"index"`
	BillingSource     string `json:"billing_source" gorm:"type:varchar(32)"`
	AttemptId         string `json:"attempt_id" gorm:"type:varchar(36)"`
	Undone            bool   `json:"undone"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint"`
}

func (l *TaskBillingLedger) BeforeCreate(tx *gorm.DB) error {
	l.CreatedAt = common.GetTimestamp()
	return nil
}

type TaskBillingStageParams struct {
	TaskType          string
	TaskRecordId      int64
	Operation         string
	Stage             string
	Delta             int // positive consumes quota; negative refunds quota
	TargetQuota       int // used by the finalize stage
	RequestCountDelta int
	UserId            int
	TokenId           int
	TokenKey          string
	ChannelId         int
	SubscriptionId    int
	BillingSource     string
	SubmissionBilling *TaskSubmissionBilling // aggregate-baseline recovery snapshot; not part of stage identity
	OperationFence    string                 // terminal-operation cache owner; not part of durable stage identity
}

func validateTaskBillingStageParams(p TaskBillingStageParams) error {
	if p.TaskRecordId <= 0 || p.Operation == "" {
		return errors.New("invalid task billing stage identity")
	}
	if p.TaskType != TaskBillingTypeTask && p.TaskType != TaskBillingTypeMidjourney {
		return fmt.Errorf("unsupported task billing type %q", p.TaskType)
	}
	if p.Stage != TaskBillingStageFunding && p.Stage != TaskBillingStageToken &&
		p.Stage != TaskBillingStageFinalize && p.Stage != TaskBillingStageAggregateBaseline {
		return fmt.Errorf("unsupported task billing stage %q", p.Stage)
	}
	if p.Stage == TaskBillingStageAggregateBaseline {
		if p.TaskType != TaskBillingTypeTask {
			return errors.New("aggregate baseline is only supported for task billing")
		}
		if p.Delta != p.TargetQuota || p.RequestCountDelta != 1 || p.UserId <= 0 {
			return errors.New("invalid task aggregate baseline")
		}
	}
	if p.Stage == TaskBillingStageToken && p.Delta != 0 && p.TokenId <= 0 {
		return errors.New("token billing stage requires token id")
	}
	if p.TargetQuota < 0 || p.TargetQuota > common.MaxQuota {
		return fmt.Errorf("task billing target quota out of range: %d", p.TargetQuota)
	}
	if p.RequestCountDelta < 0 {
		return fmt.Errorf("task billing request count delta cannot be negative: %d", p.RequestCountDelta)
	}
	return nil
}

func lockTaskBillingRecord(tx *gorm.DB, taskType string, id int64) error {
	switch taskType {
	case TaskBillingTypeTask:
		var task Task
		return lockForUpdate(tx).Select("id").Where("id = ?", id).First(&task).Error
	case TaskBillingTypeMidjourney:
		var task Midjourney
		return lockForUpdate(tx).Select("id").Where("id = ?", id).First(&task).Error
	default:
		return fmt.Errorf("unsupported task billing type %q", taskType)
	}
}

// GetTaskBillingStage returns an applied stage. A missing row is reported as
// (nil, nil), which is useful for legacy tasks created before the journal.
func GetTaskBillingStage(taskType string, taskRecordId int64, operation, stage string) (*TaskBillingLedger, error) {
	ledger, err := getTaskBillingStage(DB, taskType, taskRecordId, operation, stage)
	if err != nil {
		return nil, err
	}
	if ledger == nil || ledger.Undone {
		return nil, nil
	}
	return ledger, nil
}

// GetTaskBillingStageRecord returns the journal row even when it has been
// compensated. Callers use this to distinguish pre-journal legacy tasks from
// a modern operation whose applied balance is now intentionally zero.
func GetTaskBillingStageRecord(taskType string, taskRecordId int64, operation, stage string) (*TaskBillingLedger, error) {
	return getTaskBillingStage(DB, taskType, taskRecordId, operation, stage)
}

// GetTaskBillingOperationRecords returns one statement-consistent journal
// snapshot. Multi-stage readers must not assemble this state with separate
// queries because another settlement can commit between those reads.
func GetTaskBillingOperationRecords(taskType string, taskRecordId int64, operation string) ([]TaskBillingLedger, error) {
	var ledgers []TaskBillingLedger
	err := DB.Where("task_type = ? AND task_record_id = ? AND operation = ?", taskType, taskRecordId, operation).
		Order("id ASC").Find(&ledgers).Error
	return ledgers, err
}

// ApplyTaskBillingStage atomically applies one accounting stage and journals
// it. The boolean is false when this exact stage was already committed.
func ApplyTaskBillingStage(p TaskBillingStageParams) (bool, error) {
	if err := validateTaskBillingStageParams(p); err != nil {
		return false, errors.Join(err, ErrTaskBillingStageNotCommitted)
	}
	cacheTarget, cacheErr := prepareTaskBillingCacheDebit(p)
	if cacheErr != nil && cacheTarget.kind != "" {
		ledger, ledgerErr := getTaskBillingStage(DB, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
		if ledgerErr != nil {
			return false, errors.Join(cacheErr, ledgerErr, ErrTaskBillingStageNotCommitted)
		}
		if ledger != nil {
			if !taskBillingStageMatches(ledger, p) {
				return false, errors.Join(ErrTaskBillingStageConflict, ErrTaskBillingStageNotCommitted)
			}
			mode := "apply"
			if ledger.Undone {
				mode = "undo"
			}
			if reconcileErr := reconcileTaskBillingCacheUncertainty(cacheTarget, p, mode, ledger.AttemptId); reconcileErr != nil {
				return false, errors.Join(cacheErr, reconcileErr, ErrTaskBillingStageNotCommitted)
			}
			if !ledger.Undone {
				return false, nil
			}
			cacheTarget, cacheErr = prepareTaskBillingCacheDebit(p)
		}
	}
	if cacheErr != nil {
		return false, errors.Join(cacheErr, ErrTaskBillingStageNotCommitted)
	}
	attemptId := common.GetUUID()
	cacheDebited := false
	transactionCallbackCompleted := false
	duplicateAttemptId := ""

	err := runTaskBillingTransaction(func(tx *gorm.DB) error {
		if err := lockTaskBillingRecord(tx, p.TaskType, p.TaskRecordId); err != nil {
			return err
		}
		if err := validateTaskBillingCrossOperationState(tx, p); err != nil {
			return err
		}
		ledger, err := getTaskBillingStage(tx, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
		if err != nil {
			return err
		}
		if err := validateTaskBillingCrossOperationState(tx, p); err != nil {
			return err
		}
		if ledger != nil && !ledger.Undone {
			if !taskBillingStageMatches(ledger, p) {
				return ErrTaskBillingStageConflict
			}
			duplicateAttemptId = ledger.AttemptId
			return errTaskBillingStageApplied
		}
		if ledger != nil && !taskBillingStageMatches(ledger, p) {
			return ErrTaskBillingStageConflict
		}
		if err := validateTaskBillingOperationState(tx, p); err != nil {
			return err
		}
		if cacheTarget.delta < 0 {
			if err := applyTaskBillingCacheDelta(cacheTarget); err != nil {
				return err
			}
			cacheDebited = true
		}

		switch p.Stage {
		case TaskBillingStageFunding:
			if err := applyTaskFundingDelta(tx, p); err != nil {
				return err
			}
		case TaskBillingStageToken:
			tokenUsedDelta, err := applyTaskTokenDelta(tx, p, nil)
			if err != nil {
				return err
			}
			cacheTarget.tokenUsedDelta = int64(tokenUsedDelta)
		case TaskBillingStageFinalize:
			if err := applyTaskFinalize(tx, p); err != nil {
				return err
			}
		case TaskBillingStageAggregateBaseline:
			tokenUsedDelta, err := applyTaskAggregateBaseline(tx, p)
			if err != nil {
				return err
			}
			cacheTarget.tokenUsedDelta = int64(tokenUsedDelta)
		}

		if ledger == nil {
			if err := tx.Create(&TaskBillingLedger{
				TaskType: p.TaskType, TaskRecordId: p.TaskRecordId, Operation: p.Operation, Stage: p.Stage,
				Delta: p.Delta, TargetQuota: p.TargetQuota, TokenUsedDelta: int(cacheTarget.tokenUsedDelta),
				RequestCountDelta: p.RequestCountDelta,
				UserId:            p.UserId, TokenId: p.TokenId, ChannelId: p.ChannelId,
				SubscriptionId: p.SubscriptionId, BillingSource: p.BillingSource,
				AttemptId: attemptId,
			}).Error; err != nil {
				return err
			}
		} else if err := tx.Model(ledger).Updates(map[string]interface{}{
			"attempt_id":       attemptId,
			"token_used_delta": int(cacheTarget.tokenUsedDelta),
			"undone":           false,
		}).Error; err != nil {
			return err
		}
		transactionCallbackCompleted = true
		return nil
	})
	if errors.Is(err, errTaskBillingStageApplied) {
		if reconcileErr := reconcileTaskBillingCacheUncertainty(cacheTarget, p, "apply", duplicateAttemptId); reconcileErr != nil {
			return false, reconcileErr
		}
		return false, nil
	}
	if err != nil {
		if !transactionCallbackCompleted {
			if cacheDebited {
				compensateTaskBillingCacheDebit(cacheTarget)
			}
			return false, errors.Join(err, ErrTaskBillingStageNotCommitted)
		}
		committed, _, resolveErr := resolveTaskBillingApplyCommitFn(p, attemptId)
		if committed {
			if cacheTarget.delta > 0 {
				syncTaskBillingCacheCredit(cacheTarget)
			}
			return true, nil
		}
		// Once the transaction callback completed, a COMMIT transport error is
		// in-doubt even when one immediate recheck sees no row. MySQL/PostgreSQL
		// may make the commit visible after that SELECT. Never restore a cache
		// debit from this path; fence the mirror and let a later retry coordinate
		// against the durable journal.
		var fenceErr error
		if cacheTarget.kind != "" {
			fenceErr = fenceTaskBillingCacheUncertainty(cacheTarget, p, "apply", attemptId)
		}
		return false, errors.Join(err, resolveErr, fenceErr, ErrTaskBillingCommitUncertain)
	}

	if cacheTarget.delta > 0 {
		syncTaskBillingCacheCredit(cacheTarget)
	}
	return true, nil
}

// UndoTaskBillingStage atomically compensates an applied funding/token stage
// and marks its journal row undone. If compensation commits, the original
// operation can be retried; if it fails, the applied marker stays authoritative.
func UndoTaskBillingStage(p TaskBillingStageParams) (bool, error) {
	if err := validateTaskBillingStageParams(p); err != nil {
		return false, err
	}
	if p.Stage == TaskBillingStageFinalize || p.Stage == TaskBillingStageAggregateBaseline {
		return false, errors.New("finalize and aggregate baseline stages cannot be undone")
	}
	cacheTarget := billingCacheTargetForStage(p)
	cacheTarget.delta = -cacheTarget.delta
	if cacheTarget.delta < 0 || cacheTarget.operationFence != "" {
		preparedTarget, prepareErr := prepareTaskBillingCacheDebitWithTarget(cacheTarget)
		if prepareErr != nil && cacheTarget.kind != "" {
			ledger, ledgerErr := getTaskBillingStage(DB, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
			if ledgerErr != nil {
				return false, errors.Join(prepareErr, ledgerErr)
			}
			if ledger != nil {
				if !taskBillingStageMatches(ledger, p) {
					return false, ErrTaskBillingStageConflict
				}
				mode := "apply"
				if ledger.Undone {
					mode = "undo"
				}
				if reconcileErr := reconcileTaskBillingCacheUncertainty(cacheTarget, p, mode, ledger.AttemptId); reconcileErr != nil {
					return false, errors.Join(prepareErr, reconcileErr)
				}
				if ledger.Undone {
					return false, nil
				}
				preparedTarget, prepareErr = prepareTaskBillingCacheDebitWithTarget(cacheTarget)
			}
		}
		if prepareErr != nil {
			return false, prepareErr
		}
		cacheTarget = preparedTarget
	}
	attemptId := common.GetUUID()
	cacheDebited := false
	transactionCallbackCompleted := false
	duplicateAttemptId := ""
	err := runTaskBillingTransaction(func(tx *gorm.DB) error {
		if err := lockTaskBillingRecord(tx, p.TaskType, p.TaskRecordId); err != nil {
			return err
		}
		ledger, err := getTaskBillingStage(tx, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
		if err != nil {
			return err
		}
		if ledger == nil {
			return errTaskBillingStageNotApplied
		}
		if ledger.Undone {
			duplicateAttemptId = ledger.AttemptId
			return errTaskBillingStageNotApplied
		}
		if !taskBillingStageMatches(ledger, p) {
			return ErrTaskBillingStageConflict
		}
		if err := validateTaskBillingUndoState(tx, p); err != nil {
			return err
		}
		inverse := taskBillingStageParamsFromLedger(ledger)
		inverse.TokenKey = p.TokenKey
		inverse.Delta = -ledger.Delta
		cacheTarget.tokenUsedDelta = -int64(ledger.TokenUsedDelta)
		if cacheTarget.delta < 0 {
			if err := applyTaskBillingCacheDelta(cacheTarget); err != nil {
				return err
			}
			cacheDebited = true
		}
		if p.Stage == TaskBillingStageFunding {
			if err := applyTaskFundingDelta(tx, inverse); err != nil {
				return err
			}
		} else {
			usedDelta := -ledger.TokenUsedDelta
			if _, err := applyTaskTokenDelta(tx, inverse, &usedDelta); err != nil {
				return err
			}
		}
		if err := tx.Model(ledger).Updates(map[string]interface{}{
			"attempt_id": attemptId,
			"undone":     true,
		}).Error; err != nil {
			return err
		}
		transactionCallbackCompleted = true
		return nil
	})
	if errors.Is(err, errTaskBillingStageNotApplied) {
		if duplicateAttemptId != "" {
			if reconcileErr := reconcileTaskBillingCacheUncertainty(cacheTarget, p, "undo", duplicateAttemptId); reconcileErr != nil {
				return false, reconcileErr
			}
		}
		return false, nil
	}
	if err != nil {
		if !transactionCallbackCompleted {
			if cacheDebited {
				compensateTaskBillingCacheDebit(cacheTarget)
			}
			return false, errors.Join(err, ErrTaskBillingStageNotCommitted)
		}
		committed, _, resolveErr := resolveTaskBillingUndoCommitFn(p, attemptId)
		if committed {
			if cacheTarget.delta > 0 {
				syncTaskBillingCacheCredit(cacheTarget)
			}
			return true, nil
		}
		var fenceErr error
		if cacheTarget.kind != "" {
			fenceErr = fenceTaskBillingCacheUncertainty(cacheTarget, p, "undo", attemptId)
		}
		return false, errors.Join(err, resolveErr, fenceErr, ErrTaskBillingCommitUncertain)
	}
	if cacheTarget.delta > 0 {
		syncTaskBillingCacheCredit(cacheTarget)
	}
	return true, nil
}

func validateTaskBillingUndoState(tx *gorm.DB, p TaskBillingStageParams) error {
	var laterStageCount int64
	query := tx.Model(&TaskBillingLedger{}).
		Where("task_type = ? AND task_record_id = ? AND operation = ? AND undone = ?",
			p.TaskType, p.TaskRecordId, p.Operation, false)
	switch p.Stage {
	case TaskBillingStageFunding:
		query = query.Where("stage IN ?", []string{TaskBillingStageToken, TaskBillingStageFinalize})
	case TaskBillingStageToken:
		query = query.Where("stage = ?", TaskBillingStageFinalize)
	default:
		return nil
	}
	if err := query.Count(&laterStageCount).Error; err != nil {
		return err
	}
	if laterStageCount != 0 {
		return fmt.Errorf("%w: a later billing stage is still applied", ErrTaskBillingOperationPending)
	}
	return nil
}

var (
	ErrTaskBillingStageConflict     = errors.New("task billing stage conflicts with its durable journal")
	ErrTaskBillingOperationPending  = errors.New("another task billing operation is pending")
	ErrTaskBillingQuotaChanged      = errors.New("task quota changed before billing operation")
	ErrTaskBillingCommitUncertain   = errors.New("task billing commit outcome is uncertain")
	ErrTaskBillingStageNotCommitted = errors.New("task billing stage did not commit")
	errTaskBillingStageApplied      = errors.New("task billing stage already applied")
	errTaskBillingStageNotApplied   = errors.New("task billing stage is not applied")

	// These indirections keep commit-outcome ambiguity deterministic in tests.
	// Production always uses GORM's transaction and the durable-ledger recheck.
	runTaskBillingTransaction       = func(fn func(*gorm.DB) error) error { return DB.Transaction(fn) }
	resolveTaskBillingApplyCommitFn = resolveTaskBillingApplyCommit
	resolveTaskBillingUndoCommitFn  = resolveTaskBillingUndoCommit
)

type taskBillingCacheMutation struct {
	kind           string
	userId         int
	tokenId        int
	tokenKey       string
	delta          int64
	tokenUsedDelta int64
	operationFence string
}

const (
	taskBillingCacheUser  = "user"
	taskBillingCacheToken = "token"
)

func getTaskBillingStage(tx *gorm.DB, taskType string, taskRecordId int64, operation, stage string) (*TaskBillingLedger, error) {
	var ledger TaskBillingLedger
	result := tx.Where("task_type = ? AND task_record_id = ? AND operation = ? AND stage = ?",
		taskType, taskRecordId, operation, stage).Limit(1).Find(&ledger)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &ledger, nil
}

func taskBillingStageMatches(ledger *TaskBillingLedger, p TaskBillingStageParams) bool {
	return ledger != nil &&
		ledger.TaskType == p.TaskType &&
		ledger.TaskRecordId == p.TaskRecordId &&
		ledger.Operation == p.Operation &&
		ledger.Stage == p.Stage &&
		ledger.Delta == p.Delta &&
		ledger.TargetQuota == p.TargetQuota &&
		ledger.RequestCountDelta == p.RequestCountDelta &&
		ledger.UserId == p.UserId &&
		ledger.TokenId == p.TokenId &&
		ledger.ChannelId == p.ChannelId &&
		ledger.SubscriptionId == p.SubscriptionId &&
		ledger.BillingSource == p.BillingSource
}

func taskBillingOperationMatches(ledger *TaskBillingLedger, p TaskBillingStageParams) bool {
	return ledger != nil &&
		ledger.TaskType == p.TaskType &&
		ledger.TaskRecordId == p.TaskRecordId &&
		ledger.Operation == p.Operation &&
		ledger.Delta == p.Delta &&
		ledger.TargetQuota == p.TargetQuota &&
		ledger.RequestCountDelta == p.RequestCountDelta &&
		ledger.UserId == p.UserId &&
		ledger.TokenId == p.TokenId &&
		ledger.ChannelId == p.ChannelId &&
		ledger.SubscriptionId == p.SubscriptionId &&
		ledger.BillingSource == p.BillingSource
}

func taskBillingStageParamsFromLedger(ledger *TaskBillingLedger) TaskBillingStageParams {
	return TaskBillingStageParams{
		TaskType: ledger.TaskType, TaskRecordId: ledger.TaskRecordId, Operation: ledger.Operation,
		Stage: ledger.Stage, Delta: ledger.Delta, TargetQuota: ledger.TargetQuota,
		RequestCountDelta: ledger.RequestCountDelta, UserId: ledger.UserId, TokenId: ledger.TokenId,
		ChannelId: ledger.ChannelId, SubscriptionId: ledger.SubscriptionId, BillingSource: ledger.BillingSource,
	}
}

// validateTaskBillingCrossOperationState prevents a callback/refund from
// interleaving with Midjourney's three-stage submit settlement. The task row is
// already locked by the caller on databases that support row locks; SQLite's
// writer serialization provides the corresponding fail-closed behavior.
func validateTaskBillingCrossOperationState(tx *gorm.DB, p TaskBillingStageParams) error {
	if p.TaskType == TaskBillingTypeTask {
		if p.Stage == TaskBillingStageAggregateBaseline {
			return nil
		}
		var task Task
		if err := tx.Select("id", "billing_status").Where("id = ?", p.TaskRecordId).First(&task).Error; err != nil {
			return err
		}
		operationFenceMatches := false
		if task.BillingStatus == TaskBillingStatusReconciling && p.OperationFence != "" {
			fenceTaskType, fenceTaskId, fenceOperation, fenceErr := parseTaskBillingOperationFenceValue(p.OperationFence)
			operationFenceMatches = fenceErr == nil && fenceTaskType == p.TaskType &&
				fenceTaskId == p.TaskRecordId && fenceOperation == p.Operation
		}
		if !task.BillingReady() && !operationFenceMatches {
			return fmt.Errorf("%w: task billing status is %s", ErrTaskBillingOperationPending, task.BillingStatus)
		}
		return nil
	}
	if p.TaskType != TaskBillingTypeMidjourney {
		return nil
	}

	var ledgers []TaskBillingLedger
	if err := tx.Where("task_type = ? AND task_record_id = ?", p.TaskType, p.TaskRecordId).Find(&ledgers).Error; err != nil {
		return err
	}

	submitSeen := false
	submitFundingApplied := false
	submitFinalized := false
	refundSeen := false
	for _, ledger := range ledgers {
		switch ledger.Operation {
		case "submit":
			submitSeen = true
			if ledger.Stage == TaskBillingStageFunding && !ledger.Undone {
				submitFundingApplied = true
			}
			if ledger.Stage == TaskBillingStageFinalize && !ledger.Undone {
				submitFinalized = true
			}
		case "refund":
			refundSeen = true
		}
	}

	switch p.Operation {
	case "submit":
		if refundSeen {
			return fmt.Errorf("%w: Midjourney refund already started", ErrTaskBillingOperationPending)
		}
	case "refund":
		if submitSeen {
			if !submitFundingApplied || !submitFinalized {
				return fmt.Errorf("%w: Midjourney submit settlement is incomplete", ErrTaskBillingOperationPending)
			}
			return nil
		}

		// A nonzero billing channel marks rows created by the journal-aware
		// submit flow. With no submit ledger yet, the charge is pending rather
		// than a legacy balance that may be refunded.
		var task Midjourney
		if err := tx.Select("id", "billing_channel_id").Where("id = ?", p.TaskRecordId).First(&task).Error; err != nil {
			return err
		}
		if task.BillingChannelId > 0 {
			return fmt.Errorf("%w: Midjourney submit settlement has not started", ErrTaskBillingOperationPending)
		}
	}
	return nil
}

func validateTaskBillingOperationState(tx *gorm.DB, p TaskBillingStageParams) error {
	if p.Stage == TaskBillingStageAggregateBaseline {
		var laterStageCount int64
		if err := tx.Model(&TaskBillingLedger{}).
			Where("task_type = ? AND task_record_id = ? AND stage <> ?",
				p.TaskType, p.TaskRecordId, TaskBillingStageAggregateBaseline).
			Count(&laterStageCount).Error; err != nil {
			return err
		}
		if laterStageCount != 0 {
			return fmt.Errorf("%w: task settlement started before aggregate baseline", ErrTaskBillingOperationPending)
		}
		return nil
	}
	if p.Stage != TaskBillingStageFunding {
		funding, err := getTaskBillingStage(tx, p.TaskType, p.TaskRecordId, p.Operation, TaskBillingStageFunding)
		if err != nil {
			return err
		}
		if funding == nil || funding.Undone {
			return fmt.Errorf("%w: funding stage is not applied", ErrTaskBillingOperationPending)
		}
		if !taskBillingOperationMatches(funding, p) {
			return ErrTaskBillingStageConflict
		}
		return nil
	}
	if p.TaskType != TaskBillingTypeTask {
		return nil
	}

	var ledgers []TaskBillingLedger
	if err := tx.Where("task_type = ? AND task_record_id = ? AND undone = ?",
		p.TaskType, p.TaskRecordId, false).Find(&ledgers).Error; err != nil {
		return err
	}
	finalized := make(map[string]bool, len(ledgers))
	for _, ledger := range ledgers {
		if ledger.Stage == TaskBillingStageFinalize {
			finalized[ledger.Operation] = true
		}
	}
	for _, ledger := range ledgers {
		if ledger.Stage == TaskBillingStageAggregateBaseline {
			continue
		}
		if ledger.Operation != p.Operation && ledger.Stage != TaskBillingStageFinalize && !finalized[ledger.Operation] {
			return ErrTaskBillingOperationPending
		}
		if ledger.Operation == p.Operation {
			return ErrTaskBillingStageConflict
		}
	}

	var task Task
	if err := tx.Select("id", "quota").Where("id = ?", p.TaskRecordId).First(&task).Error; err != nil {
		return err
	}
	expectedDelta := int64(p.TargetQuota) - int64(task.Quota)
	if expectedDelta != int64(p.Delta) {
		return fmt.Errorf("%w: current=%d target=%d delta=%d", ErrTaskBillingQuotaChanged, task.Quota, p.TargetQuota, p.Delta)
	}
	return nil
}

func resolveTaskBillingApplyCommit(p TaskBillingStageParams, attemptId string) (committed, duplicate bool, err error) {
	ledger, err := getTaskBillingStage(DB, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
	if err != nil {
		return false, false, fmt.Errorf("resolve task billing apply commit: %w", err)
	}
	if ledger == nil || ledger.Undone {
		return false, false, nil
	}
	if !taskBillingStageMatches(ledger, p) {
		return false, false, ErrTaskBillingStageConflict
	}
	if ledger.AttemptId == attemptId {
		return true, false, nil
	}
	return false, true, nil
}

func resolveTaskBillingUndoCommit(p TaskBillingStageParams, attemptId string) (committed, duplicate bool, err error) {
	ledger, err := getTaskBillingStage(DB, p.TaskType, p.TaskRecordId, p.Operation, p.Stage)
	if err != nil {
		return false, false, fmt.Errorf("resolve task billing undo commit: %w", err)
	}
	if ledger == nil {
		return false, false, nil
	}
	if !taskBillingStageMatches(ledger, p) {
		return false, false, ErrTaskBillingStageConflict
	}
	if !ledger.Undone {
		return false, false, nil
	}
	if ledger.AttemptId == attemptId {
		return true, false, nil
	}
	return false, true, nil
}

func billingCacheTargetForStage(p TaskBillingStageParams) taskBillingCacheMutation {
	if p.Delta == 0 {
		return taskBillingCacheMutation{}
	}
	if p.Stage == TaskBillingStageFunding && p.BillingSource != "subscription" {
		return taskBillingCacheMutation{
			kind: taskBillingCacheUser, userId: p.UserId, delta: -int64(p.Delta),
			operationFence: p.OperationFence,
		}
	}
	if p.Stage == TaskBillingStageToken {
		return taskBillingCacheMutation{
			kind: taskBillingCacheToken, tokenId: p.TokenId, tokenKey: p.TokenKey,
			delta: -int64(p.Delta), tokenUsedDelta: int64(p.Delta),
			operationFence: p.OperationFence,
		}
	}
	return taskBillingCacheMutation{}
}

func prepareTaskBillingCacheDebit(p TaskBillingStageParams) (taskBillingCacheMutation, error) {
	target := billingCacheTargetForStage(p)
	return prepareTaskBillingCacheDebitWithTarget(target)
}

func prepareTaskBillingCacheDebitWithTarget(target taskBillingCacheMutation) (taskBillingCacheMutation, error) {
	if target.kind == "" || !common.RedisEnabled {
		return target, nil
	}
	if target.operationFence != "" {
		if err := ensureTaskBillingOperationCacheAvailable(target); err != nil {
			return target, err
		}
		return target, nil
	}
	if target.delta >= 0 {
		return target, nil
	}
	if target.kind == taskBillingCacheUser {
		if err := ensureUserQuotaCacheAvailable(target.userId); err != nil {
			return target, err
		}
		return target, nil
	}
	if target.kind == taskBillingCacheToken {
		if err := resolveTaskBillingTokenQuotaFences(target.tokenKey, target.tokenId); err != nil {
			return target, err
		}
	}

	result, err := inspectTaskBillingCacheTarget(target)
	if err == nil && result == cacheQuotaMiss {
		switch target.kind {
		case taskBillingCacheToken:
			if hasPendingBatchUpdate(BatchUpdateTypeTokenQuota, target.tokenId) {
				return target, fmt.Errorf("%w: token %d has pending deltas", ErrQuotaCacheUnavailable, target.tokenId)
			}
			_, err = GetTokenByKey(target.tokenKey, true)
		}
		if err == nil {
			result, err = inspectTaskBillingCacheTarget(target)
		}
	}
	if err != nil || result != cacheQuotaOK {
		return target, fmt.Errorf("%w: prepare %s quota cache", ErrQuotaCacheUnavailable, target.kind)
	}
	return target, nil
}

func ensureTaskBillingOperationCacheAvailable(target taskBillingCacheMutation) error {
	result, err := inspectTaskBillingCacheTarget(target)
	if err == nil && result == cacheQuotaOK {
		return nil
	}
	var fenceKey string
	switch target.kind {
	case taskBillingCacheUser:
		fenceKey = getTaskBillingUserQuotaFenceKey(target.userId)
	case taskBillingCacheToken:
		fenceKey = getTaskBillingTokenQuotaFenceKey(target.tokenKey)
	default:
		return nil
	}
	fenceValues, fenceErr := common.RDB.SMembers(context.Background(), fenceKey).Result()
	if fenceErr != nil || len(fenceValues) == 0 {
		return fmt.Errorf("%w: %s task billing operation fence changed", ErrQuotaCacheUnavailable, target.kind)
	}
	ownerFound := false
	for _, fenceValue := range fenceValues {
		if fenceValue == target.operationFence {
			ownerFound = true
		}
		if !strings.HasPrefix(fenceValue, "inflight:task-billing-operation|") {
			return fmt.Errorf("%w: %s task billing commit attempt is unresolved", ErrQuotaCacheUnavailable, target.kind)
		}
	}
	if !ownerFound {
		return fmt.Errorf("%w: %s task billing operation fence ownership changed", ErrQuotaCacheUnavailable, target.kind)
	}
	switch target.kind {
	case taskBillingCacheUser:
		var user User
		if err := DB.Where("id = ?", target.userId).First(&user).Error; err != nil {
			return fmt.Errorf("%w: hydrate user %d under task operation fence: %v", ErrQuotaCacheUnavailable, target.userId, err)
		}
		if err := populateUserCacheWhileTaskBillingFences(user, fenceValues); err != nil {
			return err
		}
	case taskBillingCacheToken:
		var token Token
		if err := DB.Where("id = ? AND "+commonKeyCol+" = ?", target.tokenId, target.tokenKey).First(&token).Error; err != nil {
			return fmt.Errorf("%w: hydrate token %d under task operation fence: %v", ErrQuotaCacheUnavailable, target.tokenId, err)
		}
		if _, err := populateTokenCacheWhileTaskBillingFences(token, fenceValues); err != nil {
			return err
		}
	}
	result, err = inspectTaskBillingCacheTarget(target)
	if err != nil || result != cacheQuotaOK {
		return fmt.Errorf("%w: hydrate %s task billing operation cache", ErrQuotaCacheUnavailable, target.kind)
	}
	return nil
}

func inspectTaskBillingCacheTarget(target taskBillingCacheMutation) (cacheQuotaResult, error) {
	target.delta = 0
	target.tokenUsedDelta = 0
	switch target.kind {
	case taskBillingCacheUser:
		return cacheApplyTaskUserQuotaDelta(target)
	case taskBillingCacheToken:
		if target.tokenKey == "" {
			return cacheQuotaMiss, errors.New("token key is empty")
		}
		return cacheApplyTaskTokenQuotaDelta(target)
	default:
		return cacheQuotaOK, nil
	}
}

const taskUserQuotaDeltaScript = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return -2
end
local task_fence_count = redis.call('SCARD', KEYS[3])
if task_fence_count > 0 then
  if ARGV[4] == '' or redis.call('SISMEMBER', KEYS[3], ARGV[4]) == 0 then
    return -2
  end
  local members = redis.call('SMEMBERS', KEYS[3])
  for _, member in ipairs(members) do
    if string.sub(member, 1, string.len(ARGV[5])) ~= ARGV[5] then
      return -2
    end
  end
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2])
  or tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'Quota', tonumber(ARGV[1]))
return 1`

func cacheApplyTaskUserQuotaDelta(target taskBillingCacheMutation) (cacheQuotaResult, error) {
	result, err := common.RDB.Eval(context.Background(), taskUserQuotaDeltaScript,
		[]string{
			getUserCacheKey(target.userId), getUserQuotaUncertaintyKey(target.userId),
			getTaskBillingUserQuotaFenceKey(target.userId),
		}, target.delta, target.userId, userCacheSchemaVersion, target.operationFence,
		"inflight:task-billing-operation|").Int()
	return quotaResultFromLua(result, err)
}

const taskTokenQuotaDeltaScript = `
local task_fence_count = redis.call('SCARD', KEYS[2])
if task_fence_count > 0 then
  if ARGV[5] == '' or redis.call('SISMEMBER', KEYS[2], ARGV[5]) == 0 then
    return -2
  end
  local members = redis.call('SMEMBERS', KEYS[2])
  for _, member in ipairs(members) do
    if string.sub(member, 1, string.len(ARGV[6])) ~= ARGV[6] then
      return -2
    end
  end
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[3])
  or redis.call('HEXISTS', KEYS[1], 'RemainQuota') == 0
  or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
  return -1
end
redis.call('HINCRBY', KEYS[1], 'RemainQuota', tonumber(ARGV[1]))
redis.call('HINCRBY', KEYS[1], 'UsedQuota', tonumber(ARGV[2]))
redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[4])
return 1`

func cacheApplyTaskTokenQuotaDelta(target taskBillingCacheMutation) (cacheQuotaResult, error) {
	if target.tokenKey == "" {
		return cacheQuotaMiss, errors.New("token key is empty")
	}
	result, err := common.RDB.Eval(context.Background(), taskTokenQuotaDeltaScript,
		[]string{getTokenCacheKey(target.tokenKey), getTaskBillingTokenQuotaFenceKey(target.tokenKey)},
		target.delta, target.tokenUsedDelta,
		target.tokenId, common.GetTimestamp(), target.operationFence,
		"inflight:task-billing-operation|").Int()
	return quotaResultFromLua(result, err)
}

func applyTaskBillingCacheDelta(target taskBillingCacheMutation) error {
	if target.kind == "" || target.delta == 0 || !common.RedisEnabled {
		return nil
	}
	var result cacheQuotaResult
	var err error
	switch target.kind {
	case taskBillingCacheUser:
		result, err = cacheApplyTaskUserQuotaDelta(target)
	case taskBillingCacheToken:
		result, err = cacheApplyTaskTokenQuotaDelta(target)
	}
	if err != nil || result != cacheQuotaOK {
		return fmt.Errorf("%w: apply %s quota cache delta", ErrQuotaCacheUnavailable, target.kind)
	}
	return nil
}

func compensateTaskBillingCacheDebit(target taskBillingCacheMutation) {
	target.delta = -target.delta
	target.tokenUsedDelta = -target.tokenUsedDelta
	if err := applyTaskBillingCacheDelta(target); err == nil {
		return
	}
	common.SysError("failed to compensate task billing quota cache debit")
	invalidateTaskBillingCacheTarget(target)
}

func syncTaskBillingCacheCredit(target taskBillingCacheMutation) {
	if target.delta <= 0 || target.kind == "" || !common.RedisEnabled {
		return
	}
	if err := applyTaskBillingCacheDelta(target); err == nil {
		return
	}
	// A credit is safe to leave absent or stale-low, but must never be retried as
	// an accounting stage. Drop/fence the mirror best-effort so its next valid
	// read hydrates the committed database balance.
	common.SysLog("failed to sync committed task billing quota credit to cache")
	invalidateTaskBillingCacheTarget(target)
}

func taskBillingUncertaintyFenceValue(p TaskBillingStageParams, mode, attemptId string) string {
	return fmt.Sprintf("inflight:task-billing|%s|%s|%d|%s|%s",
		mode, p.TaskType, p.TaskRecordId, p.Stage, attemptId)
}

func taskBillingOperationFenceValue(p TaskBillingStageParams) string {
	return fmt.Sprintf("inflight:task-billing-operation|%s|%d|%s", p.TaskType, p.TaskRecordId, p.Operation)
}

func parseTaskBillingOperationFenceValue(fenceValue string) (string, int64, string, error) {
	const prefix = "inflight:task-billing-operation|"
	if !strings.HasPrefix(fenceValue, prefix) {
		return "", 0, "", errors.New("not a task billing operation fence")
	}
	parts := strings.Split(strings.TrimPrefix(fenceValue, prefix), "|")
	if len(parts) != 3 || (parts[0] != TaskBillingTypeTask && parts[0] != TaskBillingTypeMidjourney) || parts[2] == "" {
		return "", 0, "", errors.New("invalid task billing operation fence")
	}
	taskRecordId, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || taskRecordId <= 0 {
		return "", 0, "", errors.New("invalid task billing operation fence identity")
	}
	return parts[0], taskRecordId, parts[2], nil
}

// BeginTaskBillingOperationFences blocks generic spending while a terminal
// multi-stage operation is in flight. Task-owned cache scripts may continue
// only while this is the sole member; a COMMIT-unknown attempt adds another
// member and immediately stops all further automatic mutations.
func BeginTaskBillingOperationFences(p TaskBillingStageParams) (string, error) {
	fenceValue := taskBillingOperationFenceValue(p)
	if !common.RedisEnabled {
		return fenceValue, nil
	}
	targets := make([]taskBillingCacheMutation, 0, 2)
	if p.BillingSource != "subscription" && p.UserId > 0 {
		targets = append(targets, taskBillingCacheMutation{kind: taskBillingCacheUser, userId: p.UserId})
	}
	if p.TokenId > 0 && p.TokenKey != "" {
		targets = append(targets, taskBillingCacheMutation{
			kind: taskBillingCacheToken, tokenId: p.TokenId, tokenKey: p.TokenKey,
		})
	}
	const script = `
redis.call('SADD', KEYS[1], ARGV[1])
return 1`
	for _, target := range targets {
		var fenceKey string
		if target.kind == taskBillingCacheUser {
			fenceKey = getTaskBillingUserQuotaFenceKey(target.userId)
		} else {
			fenceKey = getTaskBillingTokenQuotaFenceKey(target.tokenKey)
		}
		if err := common.RDB.Eval(context.Background(), script, []string{fenceKey}, fenceValue).Err(); err != nil {
			return fenceValue, fmt.Errorf("%w: begin task billing operation fence: %v", ErrQuotaCacheUnavailable, err)
		}
	}
	return fenceValue, nil
}

func CompleteTaskBillingOperationFences(p TaskBillingStageParams) error {
	if !common.RedisEnabled {
		return nil
	}
	if p.BillingSource != "subscription" && p.UserId > 0 {
		if err := resolveTaskBillingUserQuotaFences(p.UserId); err != nil {
			return err
		}
	}
	if p.TokenId > 0 && p.TokenKey != "" {
		if err := resolveTaskBillingTokenQuotaFences(p.TokenKey, p.TokenId); err != nil {
			return err
		}
	}
	return nil
}

func getTaskBillingUserQuotaFenceKey(userId int) string {
	return fmt.Sprintf("task_billing:user_quota_uncertain:%d", userId)
}

func getTaskBillingTokenQuotaFenceKey(tokenKey string) string {
	return fmt.Sprintf("task_billing:token_quota_uncertain:%s", common.GenerateHMAC(tokenKey))
}

type taskBillingFenceIdentity struct {
	mode         string
	taskType     string
	taskRecordId int64
	stage        string
	attemptId    string
}

func parseTaskBillingFenceValue(fenceValue string) (taskBillingFenceIdentity, error) {
	const prefix = "inflight:task-billing|"
	if !strings.HasPrefix(fenceValue, prefix) {
		return taskBillingFenceIdentity{}, errors.New("not a task billing quota fence")
	}
	parts := strings.Split(strings.TrimPrefix(fenceValue, prefix), "|")
	if len(parts) != 5 {
		return taskBillingFenceIdentity{}, errors.New("invalid task billing quota fence")
	}
	taskRecordId, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || taskRecordId <= 0 || parts[4] == "" {
		return taskBillingFenceIdentity{}, errors.New("invalid task billing quota fence identity")
	}
	if (parts[0] != "apply" && parts[0] != "undo") ||
		(parts[1] != TaskBillingTypeTask && parts[1] != TaskBillingTypeMidjourney) {
		return taskBillingFenceIdentity{}, errors.New("invalid task billing quota fence state")
	}
	return taskBillingFenceIdentity{
		mode: parts[0], taskType: parts[1], taskRecordId: taskRecordId,
		stage: parts[3], attemptId: parts[4],
	}, nil
}

// resolveTaskBillingUserQuotaFence verifies the durable journal outcome named
// by a persistent user-quota fence. It is called by the shared quota-cache
// resolver when no task worker remains to retry a completed poll/refund. A
// missing row is deliberately not treated as rollback proof.
func resolveTaskBillingUserQuotaFence(fenceValue string, userId int) (bool, error) {
	identity, err := parseTaskBillingFenceValue(fenceValue)
	if err != nil {
		return false, err
	}
	if identity.stage != TaskBillingStageFunding {
		return false, errors.New("invalid task billing quota fence state")
	}

	var ledger TaskBillingLedger
	result := DB.Where(
		"task_type = ? AND task_record_id = ? AND stage = ? AND attempt_id = ? AND user_id = ?",
		identity.taskType, identity.taskRecordId, identity.stage, identity.attemptId, userId,
	).Limit(1).Find(&ledger)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if ledger.BillingSource == "subscription" {
		return false, errors.New("subscription stage cannot own a user quota fence")
	}
	if identity.mode == "apply" {
		return !ledger.Undone, nil
	}
	return ledger.Undone, nil
}

var resolveTaskBillingUserQuotaFenceFn = resolveTaskBillingUserQuotaFence

func resolveTaskBillingTokenQuotaFence(fenceValue, tokenKey string, expectedTokenId int) (int, bool, error) {
	identity, err := parseTaskBillingFenceValue(fenceValue)
	if err != nil {
		return 0, false, err
	}
	if identity.stage != TaskBillingStageToken {
		return 0, false, errors.New("invalid task billing token fence state")
	}
	var ledger TaskBillingLedger
	result := DB.Where(
		"task_type = ? AND task_record_id = ? AND stage = ? AND attempt_id = ?",
		identity.taskType, identity.taskRecordId, identity.stage, identity.attemptId,
	).Limit(1).Find(&ledger)
	if result.Error != nil {
		return 0, false, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, false, nil
	}
	if ledger.TokenId <= 0 || (expectedTokenId > 0 && ledger.TokenId != expectedTokenId) {
		return 0, false, errors.New("task billing token fence owner mismatch")
	}
	var tokenCount int64
	if err := DB.Model(&Token{}).Where("id = ? AND "+commonKeyCol+" = ?", ledger.TokenId, tokenKey).
		Count(&tokenCount).Error; err != nil {
		return 0, false, err
	}
	if tokenCount != 1 {
		return 0, false, errors.New("task billing token fence key mismatch")
	}
	if identity.mode == "apply" {
		return ledger.TokenId, !ledger.Undone, nil
	}
	return ledger.TokenId, ledger.Undone, nil
}

var resolveTaskBillingTokenQuotaFenceFn = resolveTaskBillingTokenQuotaFence

func resolveTaskBillingOperationFence(fenceValue, targetKind string, targetId int) (bool, error) {
	taskType, taskRecordId, operation, err := parseTaskBillingOperationFenceValue(fenceValue)
	if err != nil {
		return false, err
	}
	if taskType != TaskBillingTypeTask {
		return false, errors.New("terminal task billing operation fence has unsupported task type")
	}
	var task Task
	if err := DB.Where("id = ?", taskRecordId).First(&task).Error; err != nil {
		return false, err
	}
	if task.PrivateData.BillingOperation == nil || task.PrivateData.BillingOperation.Operation != operation {
		return false, errors.New("task billing operation snapshot mismatch")
	}
	switch targetKind {
	case taskBillingCacheUser:
		if task.UserId != targetId || task.PrivateData.BillingSource == "subscription" {
			return false, errors.New("task billing operation user owner mismatch")
		}
	case taskBillingCacheToken:
		if task.PrivateData.TokenId != targetId || !task.TokenBillingEnabled() {
			return false, errors.New("task billing operation token owner mismatch")
		}
	default:
		return false, errors.New("invalid task billing operation fence target")
	}
	ledgers, err := GetTaskBillingOperationRecords(taskType, taskRecordId, operation)
	if err != nil {
		return false, err
	}
	var funding, token, finalize *TaskBillingLedger
	for i := range ledgers {
		ledger := &ledgers[i]
		switch ledger.Stage {
		case TaskBillingStageFunding:
			funding = ledger
		case TaskBillingStageToken:
			token = ledger
		case TaskBillingStageFinalize:
			finalize = ledger
		}
	}
	if finalize != nil && !finalize.Undone {
		return true, nil
	}
	snapshot := task.PrivateData.BillingOperation
	fundingSafe := (funding != nil && funding.Undone) ||
		(funding == nil && snapshot.FundingStage == TaskBillingStageStateNotApplied)
	tokenSafe := (token != nil && token.Undone) ||
		(token == nil && (snapshot.TokenStage == TaskBillingStageStateNotApplied ||
			snapshot.TokenStage == TaskBillingStageStateNotApplicable))
	return fundingSafe && tokenSafe, nil
}

func addTaskBillingCacheUncertainty(target taskBillingCacheMutation, fenceValue string) error {
	if !common.RedisEnabled || target.kind == "" || fenceValue == "" {
		return nil
	}
	var fenceKey, cacheKey string
	switch target.kind {
	case taskBillingCacheUser:
		fenceKey = getTaskBillingUserQuotaFenceKey(target.userId)
		cacheKey = getUserCacheKey(target.userId)
	case taskBillingCacheToken:
		fenceKey = getTaskBillingTokenQuotaFenceKey(target.tokenKey)
		cacheKey = getTokenCacheKey(target.tokenKey)
	default:
		return nil
	}
	const script = `
redis.call('SADD', KEYS[1], ARGV[1])
redis.call('DEL', KEYS[2])
return 1`
	if err := common.RDB.Eval(context.Background(), script, []string{fenceKey, cacheKey}, fenceValue).Err(); err != nil {
		return fmt.Errorf("%w: persist task billing %s quota fence: %v", ErrQuotaCacheUnavailable, target.kind, err)
	}
	return nil
}

func resolveTaskBillingUserQuotaFences(userId int) error {
	if !common.RedisEnabled || userId <= 0 {
		return nil
	}
	fenceKey := getTaskBillingUserQuotaFenceKey(userId)
	fenceValues, err := common.RDB.SMembers(context.Background(), fenceKey).Result()
	if err != nil {
		return fmt.Errorf("%w: read task billing user %d fences: %v", ErrQuotaCacheUnavailable, userId, err)
	}
	if len(fenceValues) == 0 {
		return nil
	}
	for _, fenceValue := range fenceValues {
		var committed bool
		var resolveErr error
		if strings.HasPrefix(fenceValue, "inflight:task-billing-operation|") {
			committed, resolveErr = resolveTaskBillingOperationFence(fenceValue, taskBillingCacheUser, userId)
		} else {
			committed, resolveErr = resolveTaskBillingUserQuotaFenceFn(fenceValue, userId)
		}
		if resolveErr != nil || !committed {
			return fmt.Errorf("%w: user %d task billing attempt is still uncertain", ErrQuotaCacheUnavailable, userId)
		}
	}
	var user User
	if err := DB.Where("id = ?", userId).First(&user).Error; err != nil {
		return fmt.Errorf("%w: reload user %d after task billing commit: %v", ErrQuotaCacheUnavailable, userId, err)
	}
	if err := populateUserCacheAfterTaskBillingFences(user, fenceValues); err != nil {
		remaining, readErr := common.RDB.Exists(context.Background(), fenceKey).Result()
		if readErr == nil && remaining == 0 {
			return nil
		}
		return fmt.Errorf("%w: reconcile user %d task billing attempts: %v", ErrQuotaCacheUnavailable, userId, err)
	}
	return nil
}

func resolveTaskBillingTokenQuotaFences(tokenKey string, expectedTokenId int) error {
	if !common.RedisEnabled || tokenKey == "" {
		return nil
	}
	fenceKey := getTaskBillingTokenQuotaFenceKey(tokenKey)
	fenceValues, err := common.RDB.SMembers(context.Background(), fenceKey).Result()
	if err != nil {
		return fmt.Errorf("%w: read task billing token fences: %v", ErrQuotaCacheUnavailable, err)
	}
	if len(fenceValues) == 0 {
		return nil
	}
	tokenId := expectedTokenId
	for _, fenceValue := range fenceValues {
		resolvedTokenId := tokenId
		var committed bool
		var resolveErr error
		if strings.HasPrefix(fenceValue, "inflight:task-billing-operation|") {
			if tokenId <= 0 {
				taskType, taskRecordId, _, parseErr := parseTaskBillingOperationFenceValue(fenceValue)
				if parseErr != nil || taskType != TaskBillingTypeTask {
					return fmt.Errorf("%w: task billing token operation owner is unknown", ErrQuotaCacheUnavailable)
				}
				var task Task
				if dbErr := DB.Select("id", "private_data", "token_charged").Where("id = ?", taskRecordId).First(&task).Error; dbErr != nil {
					return fmt.Errorf("%w: read task billing token operation owner: %v", ErrQuotaCacheUnavailable, dbErr)
				}
				if !task.TokenBillingEnabled() {
					return fmt.Errorf("%w: task billing token operation has no charged token", ErrQuotaCacheUnavailable)
				}
				tokenId = task.PrivateData.TokenId
			}
			resolvedTokenId = tokenId
			committed, resolveErr = resolveTaskBillingOperationFence(fenceValue, taskBillingCacheToken, tokenId)
		} else {
			resolvedTokenId, committed, resolveErr = resolveTaskBillingTokenQuotaFenceFn(fenceValue, tokenKey, tokenId)
		}
		if resolveErr != nil || !committed {
			return fmt.Errorf("%w: token task billing attempt is still uncertain", ErrQuotaCacheUnavailable)
		}
		if tokenId == 0 {
			tokenId = resolvedTokenId
		} else if tokenId != resolvedTokenId {
			return fmt.Errorf("%w: task billing fences name different tokens", ErrQuotaCacheUnavailable)
		}
	}
	var token Token
	if err := DB.Where("id = ? AND "+commonKeyCol+" = ?", tokenId, tokenKey).First(&token).Error; err != nil {
		return fmt.Errorf("%w: reload token %d after task billing commit: %v", ErrQuotaCacheUnavailable, tokenId, err)
	}
	if _, err := populateTokenCacheAfterTaskBillingFences(token, fenceValues); err != nil {
		remaining, readErr := common.RDB.Exists(context.Background(), fenceKey).Result()
		if readErr == nil && remaining == 0 {
			return nil
		}
		return fmt.Errorf("%w: reconcile token %d task billing attempts: %v", ErrQuotaCacheUnavailable, tokenId, err)
	}
	return nil
}

// fenceTaskBillingCacheUncertainty records every unresolved attempt in a
// persistent Redis set and removes the possibly stale authoritative mirror.
// Multiple concurrent COMMIT-unknown outcomes therefore cannot overwrite one
// another: hydration is allowed only after every set member has matching
// durable journal evidence.
func fenceTaskBillingCacheUncertainty(target taskBillingCacheMutation, p TaskBillingStageParams, mode, attemptId string) error {
	if !common.RedisEnabled || target.kind == "" {
		return nil
	}
	return addTaskBillingCacheUncertainty(target, taskBillingUncertaintyFenceValue(p, mode, attemptId))
}

// reconcileTaskBillingCacheUncertainty is called only after the transaction
// has returned the matching durable journal row. It repopulates the user hash
// while the exact attempt fence is still held, then releases that fence with a
// compare-and-delete so an unrelated mutation cannot be cleared accidentally.
func reconcileTaskBillingCacheUncertainty(target taskBillingCacheMutation, p TaskBillingStageParams, mode, attemptId string) error {
	if !common.RedisEnabled || target.kind == "" || attemptId == "" {
		return nil
	}
	if target.kind == taskBillingCacheToken {
		return resolveTaskBillingTokenQuotaFences(target.tokenKey, target.tokenId)
	}
	if err := resolveTaskBillingUserQuotaFences(target.userId); err != nil {
		return err
	}
	// Backward compatibility for exact-attempt fences written by versions that
	// predate the multi-attempt set.
	expectedFence := taskBillingUncertaintyFenceValue(p, mode, attemptId)
	actualFence, err := common.RDB.Get(context.Background(), getUserQuotaUncertaintyKey(target.userId)).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil || actualFence != expectedFence {
		return fmt.Errorf("%w: user %d has a different quota uncertainty fence", ErrQuotaCacheUnavailable, target.userId)
	}

	var user User
	if err := DB.Where("id = ?", target.userId).First(&user).Error; err != nil {
		return fmt.Errorf("%w: reload user %d after task billing retry: %v", ErrQuotaCacheUnavailable, target.userId, err)
	}
	if err := populateUserCacheUnderQuotaFence(user, expectedFence); err != nil {
		return fmt.Errorf("%w: repopulate user %d after task billing retry: %v", ErrQuotaCacheUnavailable, target.userId, err)
	}
	const releaseScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`
	released, err := common.RDB.Eval(context.Background(), releaseScript,
		[]string{getUserQuotaUncertaintyKey(target.userId)}, expectedFence).Int()
	if err != nil || released != 1 {
		return fmt.Errorf("%w: release user %d task billing uncertainty fence", ErrQuotaCacheUnavailable, target.userId)
	}
	return nil
}

func invalidateTaskBillingCacheTarget(target taskBillingCacheMutation) {
	var err error
	switch target.kind {
	case taskBillingCacheUser:
		err = invalidateUserCache(target.userId)
	case taskBillingCacheToken:
		err = invalidateTokenCacheForMutation(target.tokenKey)
	}
	if err != nil {
		common.SysError("failed to invalidate task billing quota cache: " + err.Error())
	}
}

// applyTaskAggregateBaseline records the initial user/channel aggregates for a
// newly submitted task. It intentionally bypasses the process-local batch
// updater: the aggregate mutations, task marker, and journal row must become
// visible together to pollers running on any instance.
func applyTaskAggregateBaseline(tx *gorm.DB, p TaskBillingStageParams) (int, error) {
	var task Task
	if err := tx.Where("id = ?", p.TaskRecordId).First(&task).Error; err != nil {
		return 0, err
	}
	if task.UserId != p.UserId || task.ChannelId != p.ChannelId || task.Quota != p.TargetQuota {
		return 0, ErrTaskBillingStageConflict
	}
	if task.BillingStatus == TaskBillingStatusManualReview {
		return 0, ErrTaskBillingStageConflict
	}
	expectedTokenId := p.TokenId
	if task.TokenCharged != nil {
		expectedTokenId = 0
		if *task.TokenCharged && task.PrivateData.TokenId > 0 {
			expectedTokenId = task.PrivateData.TokenId
		}
	}
	if p.TokenId != expectedTokenId {
		return 0, ErrTaskBillingStageConflict
	}
	if task.PrivateData.AggregateUsageState == TaskAggregateUsageLegacyUnknown {
		return 0, ErrTaskBillingStageConflict
	}
	if task.PrivateData.AggregateUsageState != TaskAggregateUsageAccounted {
		var user User
		if err := lockForUpdate(tx).Select("id", "used_quota", "request_count").Where("id = ?", p.UserId).First(&user).Error; err != nil {
			return 0, err
		}
		newUsed := int64(user.UsedQuota) + int64(p.Delta)
		newRequestCount := int64(user.RequestCount) + int64(p.RequestCountDelta)
		if newUsed < 0 || newUsed > int64(common.MaxQuota) ||
			newRequestCount < 0 || newRequestCount > int64(common.MaxQuota) {
			return 0, fmt.Errorf("task aggregate baseline out of range: used=%d quota=%d requests=%d",
				user.UsedQuota, p.Delta, user.RequestCount)
		}
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
			"used_quota": int(newUsed), "request_count": int(newRequestCount),
		}).Error; err != nil {
			return 0, err
		}

		if p.ChannelId > 0 && p.Delta != 0 {
			var channel Channel
			if err := lockForUpdate(tx).Select("id", "used_quota").Where("id = ?", p.ChannelId).First(&channel).Error; err != nil {
				return 0, err
			}
			newChannelUsed := channel.UsedQuota + int64(p.Delta)
			if newChannelUsed < channel.UsedQuota || newChannelUsed < 0 {
				return 0, fmt.Errorf("channel aggregate baseline out of range: used=%d quota=%d", channel.UsedQuota, p.Delta)
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("used_quota", newChannelUsed).Error; err != nil {
				return 0, err
			}
		}

	}
	task.PrivateData.AggregateUsageState = TaskAggregateUsageAccounted
	if p.SubmissionBilling != nil {
		snapshot := *p.SubmissionBilling
		snapshot.Failure = ""
		snapshot.FundingUncertain = false
		snapshot.TokenUncertain = false
		snapshot.UpdatedAt = common.GetTimestamp()
		task.PrivateData.SubmissionBilling = &snapshot
	}
	if err := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"private_data": task.PrivateData, "billing_status": TaskBillingStatusReady,
	}).Error; err != nil {
		return 0, err
	}
	if expectedTokenId > 0 {
		return p.Delta, nil
	}
	return 0, nil
}

func applyTaskFundingDelta(tx *gorm.DB, p TaskBillingStageParams) error {
	if p.Delta == 0 {
		return nil
	}
	if p.BillingSource == "subscription" {
		if p.SubscriptionId <= 0 {
			return errors.New("subscription billing stage requires subscription id")
		}
		var sub UserSubscription
		if err := lockForUpdate(tx).Where("id = ?", p.SubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		newUsed := sub.AmountUsed + int64(p.Delta)
		if newUsed < 0 || newUsed > sub.AmountTotal {
			return fmt.Errorf("subscription quota delta out of range: used=%d delta=%d total=%d", sub.AmountUsed, p.Delta, sub.AmountTotal)
		}
		return tx.Model(&UserSubscription{}).Where("id = ?", sub.Id).Update("amount_used", newUsed).Error
	}

	var user User
	if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", p.UserId).First(&user).Error; err != nil {
		return err
	}
	newQuota := int64(user.Quota) - int64(p.Delta)
	if newQuota < int64(common.MinQuota) || newQuota > int64(common.MaxQuota) {
		return fmt.Errorf("user quota delta out of range: quota=%d delta=%d", user.Quota, p.Delta)
	}
	return tx.Model(&User{}).Where("id = ?", user.Id).Update("quota", int(newQuota)).Error
}

func applyTaskTokenDelta(tx *gorm.DB, p TaskBillingStageParams, usedDeltaOverride *int) (int, error) {
	var task *Task
	if p.TaskType == TaskBillingTypeTask {
		var current Task
		if err := tx.Select("id", "private_data", "token_charged").Where("id = ?", p.TaskRecordId).First(&current).Error; err != nil {
			return 0, err
		}
		if current.TokenCharged != nil {
			if !*current.TokenCharged || current.PrivateData.TokenId <= 0 || current.PrivateData.TokenId != p.TokenId {
				return 0, ErrTaskBillingStageConflict
			}
		}
		task = &current
	}
	if p.Delta == 0 && (usedDeltaOverride == nil || *usedDeltaOverride == 0) {
		return 0, nil
	}
	var token Token
	if err := lockForUpdate(tx).Select("id", "remain_quota", "used_quota").Where("id = ?", p.TokenId).First(&token).Error; err != nil {
		return 0, err
	}
	usedDelta := int64(p.Delta)
	if usedDeltaOverride != nil {
		usedDelta = int64(*usedDeltaOverride)
	} else if task != nil {
		if task.PrivateData.AggregateUsageState != TaskAggregateUsageAccounted {
			// Old rows do not prove whether this task contributed to UsedQuota.
			// Adjust RemainQuota exactly, but never guess against a shared counter.
			usedDelta = 0
		}
	}
	minTokenQuota := int64(common.MinQuota)
	maxTokenQuota := int64(^uint(0) >> 1)
	remainQuota := int64(token.RemainQuota)
	usedQuota := int64(token.UsedQuota)
	delta := int64(p.Delta)
	if remainQuota < minTokenQuota || remainQuota > maxTokenQuota ||
		usedQuota < 0 || usedQuota > maxTokenQuota ||
		delta < int64(common.MinQuota) || delta > int64(common.MaxQuota) ||
		usedDelta < int64(common.MinQuota) || usedDelta > int64(common.MaxQuota) ||
		(delta > 0 && remainQuota < minTokenQuota+delta) ||
		(delta < 0 && remainQuota > maxTokenQuota+delta) ||
		(usedDelta > 0 && usedQuota > maxTokenQuota-usedDelta) ||
		(usedDelta < 0 && usedQuota < -usedDelta) {
		return 0, fmt.Errorf("token quota delta out of range: remain=%d used=%d delta=%d used_delta=%d", token.RemainQuota, token.UsedQuota, p.Delta, usedDelta)
	}
	newRemain := remainQuota - delta
	newUsed := usedQuota + usedDelta
	if newRemain < minTokenQuota || newRemain > maxTokenQuota || newUsed < 0 || newUsed > maxTokenQuota {
		return 0, fmt.Errorf("token quota delta out of range: remain=%d used=%d delta=%d used_delta=%d", token.RemainQuota, token.UsedQuota, p.Delta, usedDelta)
	}
	if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"remain_quota": int(newRemain), "used_quota": int(newUsed), "accessed_time": common.GetTimestamp(),
	}).Error; err != nil {
		return 0, err
	}
	return int(usedDelta), nil
}

func applyTaskFinalize(tx *gorm.DB, p TaskBillingStageParams) error {
	usageDelta := int64(p.Delta)
	var task *Task
	if p.TaskType == TaskBillingTypeTask {
		var current Task
		if err := tx.Where("id = ?", p.TaskRecordId).First(&current).Error; err != nil {
			return err
		}
		task = &current
		if current.PrivateData.AggregateUsageState != TaskAggregateUsageAccounted {
			usageDelta = 0
		}
	}
	if usageDelta != 0 || p.RequestCountDelta != 0 {
		var user User
		if err := lockForUpdate(tx).Select("id", "used_quota", "request_count").Where("id = ?", p.UserId).First(&user).Error; err != nil {
			return err
		}
		newUsed := int64(user.UsedQuota) + usageDelta
		if newUsed < 0 || newUsed > int64(common.MaxQuota) {
			return fmt.Errorf("user used quota delta out of range: used=%d delta=%d applied_delta=%d", user.UsedQuota, p.Delta, usageDelta)
		}
		newRequestCount := int64(user.RequestCount) + int64(p.RequestCountDelta)
		if newRequestCount < 0 || newRequestCount > int64(common.MaxQuota) {
			return fmt.Errorf("user request count delta out of range: count=%d delta=%d", user.RequestCount, p.RequestCountDelta)
		}
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
			"used_quota": int(newUsed), "request_count": int(newRequestCount),
		}).Error; err != nil {
			return err
		}
		if p.ChannelId > 0 && usageDelta != 0 {
			var channel Channel
			if err := lockForUpdate(tx).Select("id", "used_quota").Where("id = ?", p.ChannelId).First(&channel).Error; err != nil {
				return err
			}
			newChannelUsed := channel.UsedQuota + usageDelta
			if (usageDelta > 0 && newChannelUsed < channel.UsedQuota) ||
				(usageDelta < 0 && newChannelUsed > channel.UsedQuota) || newChannelUsed < 0 {
				return fmt.Errorf("channel used quota delta out of range: used=%d delta=%d applied_delta=%d", channel.UsedQuota, p.Delta, usageDelta)
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("used_quota", newChannelUsed).Error; err != nil {
				return err
			}
		}
	}

	switch p.TaskType {
	case TaskBillingTypeTask:
		updates := map[string]interface{}{"quota": p.TargetQuota}
		if task.PrivateData.AggregateUsageState == "" {
			task.PrivateData.AggregateUsageState = TaskAggregateUsageLegacyUnknown
			updates["private_data"] = task.PrivateData
		}
		return tx.Model(&Task{}).Where("id = ?", p.TaskRecordId).Updates(updates).Error
	case TaskBillingTypeMidjourney:
		return tx.Model(&Midjourney{}).Where("id = ?", p.TaskRecordId).Update("quota", p.TargetQuota).Error
	default:
		return fmt.Errorf("unsupported task billing type %q", p.TaskType)
	}
}
