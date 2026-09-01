package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useTaskLedgerRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	oldEnabled := common.RedisEnabled
	oldClient := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = oldEnabled
		common.RDB = oldClient
		require.NoError(t, client.Close())
	})
	return server
}

func TestApplyTaskBillingStageCommitUnknownKeepsCacheFailClosed(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1000)
	task := &Task{TaskID: "ledger-ambiguous-apply", UserId: user.Id, Quota: 100}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Stage: TaskBillingStageFunding,
		Delta: 50, TargetQuota: 150, UserId: user.Id, BillingSource: "wallet",
	}
	commitTransportErr := errors.New("commit result unavailable")
	recheckErr := ErrTaskBillingStageConflict
	oldRun := runTaskBillingTransaction
	oldResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, recheckErr
	}
	restore := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldResolve
	}
	t.Cleanup(restore)

	applied, err := ApplyTaskBillingStage(stage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, commitTransportErr)
	assert.ErrorIs(t, err, recheckErr)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	assert.Equal(t, 950, getUserQuotaFromDB(t, user.Id), "the committed debit must not be compensated")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "the ambiguous live balance must be removed")
	assert.True(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)), "later cache mutations must reconcile every durable attempt first")

	restore()
	applied, err = ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	assert.False(t, applied, "the durable stage makes the retry idempotent")
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 950, cached.Quota)
}

func TestUndoTaskBillingStageCommitUnknownKeepsCacheFailClosed(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1000)
	task := &Task{TaskID: "ledger-ambiguous-undo", UserId: user.Id, Quota: 100}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Stage: TaskBillingStageFunding,
		Delta: -50, TargetQuota: 50, UserId: user.Id, BillingSource: "wallet",
	}
	applied, err := ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, 1050, getUserQuotaFromDB(t, user.Id))

	commitTransportErr := errors.New("undo commit result unavailable")
	recheckErr := ErrTaskBillingStageConflict
	oldRun := runTaskBillingTransaction
	oldResolve := resolveTaskBillingUndoCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingUndoCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, recheckErr
	}
	restore := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingUndoCommitFn = oldResolve
	}
	t.Cleanup(restore)

	undone, err := UndoTaskBillingStage(stage)
	assert.False(t, undone)
	assert.ErrorIs(t, err, commitTransportErr)
	assert.ErrorIs(t, err, recheckErr)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	assert.Equal(t, 1000, getUserQuotaFromDB(t, user.Id), "the committed undo debit must not be compensated")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))

	restore()
	undone, err = UndoTaskBillingStage(stage)
	require.NoError(t, err)
	assert.False(t, undone, "the durable undone marker makes the retry idempotent")
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1000, cached.Quota)
}

func TestTokenTaskBillingCommitUnknownInvalidatesAuthoritativeHash(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)
	token.UserId = user.Id
	task := &Task{
		TaskID: "ledger-ambiguous-token", UserId: user.Id, Quota: 100,
		PrivateData: TaskPrivateData{AggregateUsageState: TaskAggregateUsageAccounted},
	}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	base := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Delta: 50, TargetQuota: 150,
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	funding := base
	funding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(funding)
	require.NoError(t, err)
	require.True(t, applied)

	tokenStage := base
	tokenStage.Stage = TaskBillingStageToken
	commitTransportErr := errors.New("token commit result unavailable")
	recheckErr := errors.New("token ledger recheck unavailable")
	oldRun := runTaskBillingTransaction
	oldResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, recheckErr
	}
	restore := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldResolve
	}
	t.Cleanup(restore)

	applied, err = ApplyTaskBillingStage(tokenStage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, commitTransportErr)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 450, reloaded.RemainQuota)
	assert.Equal(t, 50, reloaded.UsedQuota)
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	assert.True(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))

	restore()
	applied, err = ApplyTaskBillingStage(tokenStage)
	require.NoError(t, err)
	assert.False(t, applied)
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 450, cachedToken.RemainQuota)
	assert.Equal(t, 50, cachedToken.UsedQuota)
}

func TestTaskBillingTokenStageSupportsLargeBalances(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large token quotas require a 64-bit server build")
	}

	truncateTables(t)
	user := createReserveTestUser(t, 1_000)
	largeQuota := int64(5_000_000_000)
	token := createReserveTestToken(t, int(largeQuota))
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)
	task := &Task{
		TaskID:       "ledger-large-token-balance",
		UserId:       user.Id,
		Quota:        100,
		TokenCharged: common.GetPointer(true),
		PrivateData: TaskPrivateData{
			TokenId: token.Id, AggregateUsageState: TaskAggregateUsageAccounted,
		},
	}
	insertTask(t, task)

	base := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Delta: 50, TargetQuota: 150,
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	funding := base
	funding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(funding)
	require.NoError(t, err)
	require.True(t, applied)

	tokenStage := base
	tokenStage.Stage = TaskBillingStageToken
	applied, err = ApplyTaskBillingStage(tokenStage)
	require.NoError(t, err)
	require.True(t, applied)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, int(largeQuota)-50, reloaded.RemainQuota)
	assert.Equal(t, 50, reloaded.UsedQuota)

	undone, err := UndoTaskBillingStage(tokenStage)
	require.NoError(t, err)
	require.True(t, undone)
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, int(largeQuota), reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
}

func TestTaskBillingDelayedVisibleWalletApplyAndUndoStayFailClosedUntilJournalRetry(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	task := &Task{TaskID: "ledger-delayed-wallet", UserId: user.Id, Quota: 100}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Stage: TaskBillingStageFunding,
		Delta: 50, TargetQuota: 150, UserId: user.Id, BillingSource: "wallet",
	}
	commitTransportErr := errors.New("wallet apply commit acknowledgement lost")
	oldRun := runTaskBillingTransaction
	oldApplyResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, nil
	}
	restoreApply := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldApplyResolve
	}
	t.Cleanup(restoreApply)

	applied, err := ApplyTaskBillingStage(stage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	assert.Equal(t, 950, getUserQuotaFromDB(t, user.Id), "a delayed-visible commit must never be cache-compensated")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))
	cached, cacheErr := GetUserCache(user.Id)
	require.NoError(t, cacheErr, "a later authenticated read can recover once the exact journal attempt is durable")
	assert.Equal(t, 950, cached.Quota)
	assert.False(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))

	restoreApply()
	applied, err = ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	assert.False(t, applied)
	cached, err = GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 950, cached.Quota)

	undoTransportErr := errors.New("wallet undo commit acknowledgement lost")
	oldUndoResolve := resolveTaskBillingUndoCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return undoTransportErr
	}
	resolveTaskBillingUndoCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, nil
	}
	restoreUndo := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingUndoCommitFn = oldUndoResolve
	}
	t.Cleanup(restoreUndo)

	undone, err := UndoTaskBillingStage(stage)
	assert.False(t, undone)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	assert.Equal(t, 1_000, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))
	cached, err = GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1_000, cached.Quota)
	assert.False(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))

	restoreUndo()
	undone, err = UndoTaskBillingStage(stage)
	require.NoError(t, err)
	assert.False(t, undone)
	cached, err = GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1_000, cached.Quota)
}

func TestTaskBillingUnknownFenceWithoutDurableAttemptNeverHydratesStaleQuota(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	task := &Task{TaskID: "ledger-missing-attempt", UserId: user.Id, Quota: 100}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Stage: TaskBillingStageFunding,
		Delta: 50, TargetQuota: 150, UserId: user.Id, BillingSource: "wallet",
	}
	fenceValue := taskBillingUncertaintyFenceValue(stage, "apply", "missing-attempt")
	require.NoError(t, fenceUserQuotaCacheUntilReconciled(user.Id, fenceValue, fenceValue))

	_, err = GetUserCache(user.Id)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.True(t, server.Exists(getUserQuotaUncertaintyKey(user.Id)))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)), "one missing journal read must not rehydrate the old higher quota")
}

func TestTaskBillingTokenFenceSurvivesLegacyTTLAndBlocksGenericReadAndReserve(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)
	task := &Task{
		TaskID: "token-fence-delayed-visible", UserId: user.Id, Quota: 100,
		TokenCharged: common.GetPointer(true),
		PrivateData:  TaskPrivateData{TokenId: token.Id, AggregateUsageState: TaskAggregateUsageAccounted},
	}
	insertTask(t, task)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Stage: TaskBillingStageToken,
		Delta: 50, TargetQuota: 150, UserId: user.Id,
		TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	attemptId := "token-delayed-attempt"
	fenceValue := taskBillingUncertaintyFenceValue(stage, "apply", attemptId)
	require.NoError(t, addTaskBillingCacheUncertainty(billingCacheTargetForStage(stage), fenceValue))

	server.FastForward(time.Duration(tokenCacheFenceSeconds+60) * time.Second)
	assert.True(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)),
		"task fence must not inherit the legacy ten-second TTL")
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	_, err = GetTokenByKey(token.Key, false)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 1, false)
	assert.False(t, reserved)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.Equal(t, 500, getTokenFromDB(t, token.Id).RemainQuota)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
			"remain_quota": 450,
			"used_quota":   50,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&TaskBillingLedger{
			TaskType: stage.TaskType, TaskRecordId: stage.TaskRecordId,
			Operation: stage.Operation, Stage: stage.Stage, Delta: stage.Delta,
			TargetQuota: stage.TargetQuota, TokenUsedDelta: stage.Delta,
			UserId: stage.UserId, TokenId: stage.TokenId, BillingSource: stage.BillingSource,
			AttemptId: attemptId,
		}).Error
	}))
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 450, cached.RemainQuota)
	assert.Equal(t, 50, cached.UsedQuota)
	assert.False(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))
}

func TestTaskBillingMultipleUnknownAttemptsMustAllResolveBeforeUserOrTokenHydrates(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"user_id": user.Id, "remain_quota": 350, "used_quota": 150,
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 850).Error)

	tasks := []*Task{
		{TaskID: "multi-attempt-a", UserId: user.Id, Quota: 100},
		{TaskID: "multi-attempt-b", UserId: user.Id, Quota: 100},
	}
	for _, task := range tasks {
		insertTask(t, task)
	}
	userStages := make([]TaskBillingStageParams, 0, 2)
	tokenStages := make([]TaskBillingStageParams, 0, 2)
	for i, task := range tasks {
		attempt := fmt.Sprintf("multi-attempt-%d", i)
		userStage := TaskBillingStageParams{
			TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
			Operation: "settle:150", Stage: TaskBillingStageFunding,
			Delta: 50, TargetQuota: 150, UserId: user.Id, BillingSource: "wallet",
		}
		tokenStage := userStage
		tokenStage.Stage = TaskBillingStageToken
		tokenStage.TokenId = token.Id
		tokenStage.TokenKey = token.Key
		userStages = append(userStages, userStage)
		tokenStages = append(tokenStages, tokenStage)
		require.NoError(t, DB.Create(&TaskBillingLedger{
			TaskType: userStage.TaskType, TaskRecordId: userStage.TaskRecordId,
			Operation: userStage.Operation, Stage: userStage.Stage, Delta: userStage.Delta,
			TargetQuota: userStage.TargetQuota, UserId: user.Id, BillingSource: "wallet",
			AttemptId: attempt,
		}).Error)
		require.NoError(t, DB.Create(&TaskBillingLedger{
			TaskType: tokenStage.TaskType, TaskRecordId: tokenStage.TaskRecordId,
			Operation: tokenStage.Operation, Stage: tokenStage.Stage, Delta: tokenStage.Delta,
			TargetQuota: tokenStage.TargetQuota, TokenUsedDelta: tokenStage.Delta,
			UserId: user.Id, TokenId: token.Id, BillingSource: "wallet",
			AttemptId: attempt,
		}).Error)
		require.NoError(t, addTaskBillingCacheUncertainty(
			billingCacheTargetForStage(userStage), taskBillingUncertaintyFenceValue(userStage, "apply", attempt)))
		require.NoError(t, addTaskBillingCacheUncertainty(
			billingCacheTargetForStage(tokenStage), taskBillingUncertaintyFenceValue(tokenStage, "apply", attempt)))
	}
	userFenceCount, err := server.SCard(getTaskBillingUserQuotaFenceKey(user.Id))
	require.NoError(t, err)
	tokenFenceCount, err := server.SCard(getTaskBillingTokenQuotaFenceKey(token.Key))
	require.NoError(t, err)
	assert.Equal(t, 2, userFenceCount)
	assert.Equal(t, 2, tokenFenceCount)

	oldUserResolve := resolveTaskBillingUserQuotaFenceFn
	oldTokenResolve := resolveTaskBillingTokenQuotaFenceFn
	resolveTaskBillingUserQuotaFenceFn = func(value string, id int) (bool, error) {
		if strings.Contains(value, "multi-attempt-1") {
			return false, nil
		}
		return oldUserResolve(value, id)
	}
	resolveTaskBillingTokenQuotaFenceFn = func(value, key string, id int) (int, bool, error) {
		if strings.Contains(value, "multi-attempt-1") {
			return id, false, nil
		}
		return oldTokenResolve(value, key, id)
	}
	restore := func() {
		resolveTaskBillingUserQuotaFenceFn = oldUserResolve
		resolveTaskBillingTokenQuotaFenceFn = oldTokenResolve
	}
	t.Cleanup(restore)

	_, err = GetUserCache(user.Id)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	_, err = GetTokenByKey(token.Key, false)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	userFenceCount, err = server.SCard(getTaskBillingUserQuotaFenceKey(user.Id))
	require.NoError(t, err)
	tokenFenceCount, err = server.SCard(getTaskBillingTokenQuotaFenceKey(token.Key))
	require.NoError(t, err)
	assert.Equal(t, 2, userFenceCount, "A must not release B")
	assert.Equal(t, 2, tokenFenceCount, "A must not release B")
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))

	restore()
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 850, cachedUser.Quota)
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 350, cachedToken.RemainQuota)
	assert.Equal(t, 150, cachedToken.UsedQuota)
	assert.False(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))
	assert.False(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))
}

func TestTaskBillingOperationFencesAllowConcurrentOwnersAndReleaseTogether(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("used_quota", 200).Error)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"user_id": user.Id, "used_quota": 200,
	}).Error)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	tasks := []*Task{
		{
			TaskID: "operation-owner-a", UserId: user.Id, Quota: 100, BillingStatus: TaskBillingStatusReady,
			TokenCharged: common.GetPointer(true),
			PrivateData: TaskPrivateData{BillingSource: "wallet", TokenId: token.Id,
				AggregateUsageState: TaskAggregateUsageAccounted},
		},
		{
			TaskID: "operation-owner-b", UserId: user.Id, Quota: 100, BillingStatus: TaskBillingStatusReady,
			TokenCharged: common.GetPointer(true),
			PrivateData: TaskPrivateData{BillingSource: "wallet", TokenId: token.Id,
				AggregateUsageState: TaskAggregateUsageAccounted},
		},
	}
	deltas := []int{10, 20}
	bases := make([]TaskBillingStageParams, len(tasks))
	for i, task := range tasks {
		insertTask(t, task)
		operation := fmt.Sprintf("settle:%d", task.Quota+deltas[i])
		snapshot := TaskBillingOperation{
			Operation: operation, PreQuota: task.Quota, TargetQuota: task.Quota + deltas[i], Delta: deltas[i],
			FundingStage: TaskBillingStageStatePending, TokenStage: TaskBillingStageStatePending,
			FinalizeStage: TaskBillingStageStatePending,
		}
		won, beginErr := task.BeginBillingReconciliation(snapshot)
		require.NoError(t, beginErr)
		require.True(t, won)
		base := TaskBillingStageParams{
			TaskType: TaskBillingTypeTask, TaskRecordId: task.ID, Operation: operation,
			Delta: deltas[i], TargetQuota: task.Quota + deltas[i], UserId: user.Id,
			TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
		}
		owner, fenceErr := BeginTaskBillingOperationFences(base)
		require.NoError(t, fenceErr)
		base.OperationFence = owner
		bases[i] = base
	}
	userFenceCount, err := server.SCard(getTaskBillingUserQuotaFenceKey(user.Id))
	require.NoError(t, err)
	tokenFenceCount, err := server.SCard(getTaskBillingTokenQuotaFenceKey(token.Key))
	require.NoError(t, err)
	assert.Equal(t, 2, userFenceCount)
	assert.Equal(t, 2, tokenFenceCount)

	for i := range bases {
		funding := bases[i]
		funding.Stage = TaskBillingStageFunding
		applied, applyErr := ApplyTaskBillingStage(funding)
		require.NoError(t, applyErr)
		require.True(t, applied)
	}
	for i := range bases {
		tokenStage := bases[i]
		tokenStage.Stage = TaskBillingStageToken
		applied, applyErr := ApplyTaskBillingStage(tokenStage)
		require.NoError(t, applyErr)
		require.True(t, applied)
	}
	for i := range bases {
		finalize := bases[i]
		finalize.Stage = TaskBillingStageFinalize
		applied, applyErr := ApplyTaskBillingStage(finalize)
		require.NoError(t, applyErr)
		require.True(t, applied)
		snapshot := TaskBillingOperation{
			Operation: bases[i].Operation, PreQuota: 100, TargetQuota: bases[i].TargetQuota, Delta: bases[i].Delta,
			FundingStage: TaskBillingStageStateApplied, TokenStage: TaskBillingStageStateApplied,
			FinalizeStage: TaskBillingStageStateApplied,
		}
		require.NoError(t, tasks[i].UpdateBillingReconciliationSnapshot(snapshot, false))
		ready, readyErr := tasks[i].CompleteBillingReconciliation()
		require.NoError(t, readyErr)
		require.True(t, ready)
		fenceErr := CompleteTaskBillingOperationFences(bases[i])
		if i == 0 {
			assert.ErrorIs(t, fenceErr, ErrQuotaCacheUnavailable,
				"the first completed owner must not release the second in-flight owner")
		} else {
			require.NoError(t, fenceErr)
		}
	}

	assert.False(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))
	assert.False(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))
	cachedUser, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 970, cachedUser.Quota)
	var reloadedUser User
	require.NoError(t, DB.First(&reloadedUser, user.Id).Error)
	assert.Equal(t, 230, reloadedUser.UsedQuota)
	cachedToken, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 470, cachedToken.RemainQuota)
	assert.Equal(t, 230, cachedToken.UsedQuota)
}

func TestTaskBillingOperationFenceStopsOtherOwnerWhenAttemptIsUnknown(t *testing.T) {
	truncateTables(t)
	useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)

	tasks := []*Task{
		{TaskID: "operation-unknown-a", UserId: user.Id, Quota: 100, BillingStatus: TaskBillingStatusReady,
			TokenCharged: common.GetPointer(true), PrivateData: TaskPrivateData{BillingSource: "wallet", TokenId: token.Id,
				AggregateUsageState: TaskAggregateUsageAccounted}},
		{TaskID: "operation-unknown-b", UserId: user.Id, Quota: 100, BillingStatus: TaskBillingStatusReady,
			TokenCharged: common.GetPointer(true), PrivateData: TaskPrivateData{BillingSource: "wallet", TokenId: token.Id,
				AggregateUsageState: TaskAggregateUsageAccounted}},
	}
	bases := make([]TaskBillingStageParams, len(tasks))
	for i, task := range tasks {
		insertTask(t, task)
		operation := "settle:110"
		won, err := task.BeginBillingReconciliation(TaskBillingOperation{
			Operation: operation, PreQuota: 100, TargetQuota: 110, Delta: 10,
			FundingStage: TaskBillingStageStatePending, TokenStage: TaskBillingStageStatePending,
			FinalizeStage: TaskBillingStageStatePending,
		})
		require.NoError(t, err)
		require.True(t, won)
		base := TaskBillingStageParams{
			TaskType: TaskBillingTypeTask, TaskRecordId: task.ID, Operation: operation,
			Delta: 10, TargetQuota: 110, UserId: user.Id, TokenId: token.Id,
			TokenKey: token.Key, BillingSource: "wallet",
		}
		owner, err := BeginTaskBillingOperationFences(base)
		require.NoError(t, err)
		base.OperationFence = owner
		bases[i] = base
	}

	unknownFunding := bases[0]
	unknownFunding.Stage = TaskBillingStageFunding
	require.NoError(t, addTaskBillingCacheUncertainty(
		billingCacheTargetForStage(unknownFunding),
		taskBillingUncertaintyFenceValue(unknownFunding, "apply", "delayed-user-attempt"),
	))
	unknownToken := bases[0]
	unknownToken.Stage = TaskBillingStageToken
	require.NoError(t, addTaskBillingCacheUncertainty(
		billingCacheTargetForStage(unknownToken),
		taskBillingUncertaintyFenceValue(unknownToken, "apply", "delayed-token-attempt"),
	))

	otherFunding := bases[1]
	otherFunding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(otherFunding)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	otherToken := bases[1]
	otherToken.Stage = TaskBillingStageToken
	applied, err = ApplyTaskBillingStage(otherToken)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	assert.Equal(t, 1_000, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 500, getTokenFromDB(t, token.Id).RemainQuota)
	_, err = GetUserCache(user.Id)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
	_, err = GetTokenByKey(token.Key, false)
	assert.ErrorIs(t, err, ErrQuotaCacheUnavailable)
}

func TestTaskBillingTokenLateWhileFencedHydrateDoesNotOverwriteEarlierDelta(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	token := createReserveTestToken(t, 500)
	staleSnapshot := token
	ownerA := taskBillingOperationFenceValue(TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: 101, Operation: "settle:110",
	})
	ownerB := taskBillingOperationFenceValue(TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: 102, Operation: "settle:120",
	})
	fenceValues := []string{ownerA, ownerB}
	fenceKey := getTaskBillingTokenQuotaFenceKey(token.Key)
	require.NoError(t, common.RDB.SAdd(context.Background(), fenceKey, ownerA, ownerB).Err())

	initialized, err := populateTokenCacheWhileTaskBillingFences(staleSnapshot, fenceValues)
	require.NoError(t, err)
	require.True(t, initialized)
	result, err := cacheApplyTaskTokenQuotaDelta(taskBillingCacheMutation{
		kind: taskBillingCacheToken, tokenId: token.Id, tokenKey: token.Key,
		delta: -10, tokenUsedDelta: 10, operationFence: ownerA,
	})
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	lateHydrate, err := populateTokenCacheWhileTaskBillingFences(staleSnapshot, fenceValues)
	require.NoError(t, err)
	require.True(t, lateHydrate)
	remain, err := common.RDB.HGet(context.Background(), getTokenCacheKey(token.Key), "RemainQuota").Int()
	require.NoError(t, err)
	used, err := common.RDB.HGet(context.Background(), getTokenCacheKey(token.Key), "UsedQuota").Int()
	require.NoError(t, err)
	assert.Equal(t, 490, remain, "B's late old database snapshot must not overwrite A's cache delta")
	assert.Equal(t, 10, used)
	fenceCount, err := server.SCard(fenceKey)
	require.NoError(t, err)
	assert.Equal(t, 2, fenceCount, "while-fenced hydration must preserve both operation owners")
}

func TestCompletedTaskBillingTokenOperationFenceRecoversWithoutExpectedTokenID(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"user_id": user.Id, "remain_quota": 450, "used_quota": 50,
	}).Error)
	operation := "settle:150"
	task := &Task{
		TaskID: "completed-token-operation-owner", UserId: user.Id, Quota: 150,
		BillingStatus: TaskBillingStatusReady, TokenCharged: common.GetPointer(true),
		PrivateData: TaskPrivateData{
			BillingSource: "wallet", TokenId: token.Id,
			AggregateUsageState: TaskAggregateUsageAccounted,
			BillingOperation: &TaskBillingOperation{
				Operation: operation, PreQuota: 100, TargetQuota: 150, Delta: 50,
				FundingStage: TaskBillingStageStateApplied, TokenStage: TaskBillingStageStateApplied,
				FinalizeStage: TaskBillingStageStateApplied,
			},
		},
	}
	insertTask(t, task)
	require.NoError(t, DB.Create(&TaskBillingLedger{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID, Operation: operation,
		Stage: TaskBillingStageFinalize, Delta: 50, TargetQuota: 150,
		UserId: user.Id, TokenId: token.Id, BillingSource: "wallet", AttemptId: "completed-finalize",
	}).Error)
	owner := taskBillingOperationFenceValue(TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID, Operation: operation,
	})
	fenceKey := getTaskBillingTokenQuotaFenceKey(token.Key)
	require.NoError(t, common.RDB.SAdd(context.Background(), fenceKey, owner).Err())
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))

	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, token.Id, cached.Id)
	assert.Equal(t, 450, cached.RemainQuota)
	assert.Equal(t, 50, cached.UsedQuota)
	assert.False(t, server.Exists(fenceKey), "a completed owner must release without a caller-supplied token id")
}

func TestTaskBillingDelayedVisibleTokenApplyAndUndoInvalidateBeforeJournalRetry(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1_000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)
	task := &Task{
		TaskID: "ledger-delayed-token", UserId: user.Id, Quota: 100,
		TokenCharged: common.GetPointer(true),
		PrivateData: TaskPrivateData{
			TokenId: token.Id, AggregateUsageState: TaskAggregateUsageAccounted,
		},
	}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	base := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:150", Delta: 50, TargetQuota: 150,
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	funding := base
	funding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(funding)
	require.NoError(t, err)
	require.True(t, applied)
	tokenStage := base
	tokenStage.Stage = TaskBillingStageToken

	commitTransportErr := errors.New("token apply commit acknowledgement lost")
	oldRun := runTaskBillingTransaction
	oldApplyResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, nil
	}
	restoreApply := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldApplyResolve
	}
	t.Cleanup(restoreApply)

	applied, err = ApplyTaskBillingStage(tokenStage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 450, reloaded.RemainQuota)
	assert.Equal(t, 50, reloaded.UsedQuota)
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	assert.True(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))

	restoreApply()
	applied, err = ApplyTaskBillingStage(tokenStage)
	require.NoError(t, err)
	assert.False(t, applied)
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 450, cached.RemainQuota)
	assert.Equal(t, 50, cached.UsedQuota)

	undoTransportErr := errors.New("token undo commit acknowledgement lost")
	oldUndoResolve := resolveTaskBillingUndoCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return undoTransportErr
	}
	resolveTaskBillingUndoCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, nil
	}
	restoreUndo := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingUndoCommitFn = oldUndoResolve
	}
	t.Cleanup(restoreUndo)

	undone, err := UndoTaskBillingStage(tokenStage)
	assert.False(t, undone)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 500, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	assert.True(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))

	restoreUndo()
	undone, err = UndoTaskBillingStage(tokenStage)
	require.NoError(t, err)
	assert.False(t, undone)
	cached, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 500, cached.RemainQuota)
	assert.Zero(t, cached.UsedQuota)
}

func TestLegacyUnknownTaskLeavesTokenUsedCounterUntouchedAndUndoableWithRedis(t *testing.T) {
	truncateTables(t)
	useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 10_000)
	token := createReserveTestToken(t, 8_000)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("user_id", user.Id).Error)
	task := &Task{TaskID: "ledger-legacy-token-rebase", UserId: user.Id, Quota: 5_000}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	base := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:3000", Delta: -2_000, TargetQuota: 3_000,
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	funding := base
	funding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(funding)
	require.NoError(t, err)
	require.True(t, applied)
	tokenStage := base
	tokenStage.Stage = TaskBillingStageToken
	applied, err = ApplyTaskBillingStage(tokenStage)
	require.NoError(t, err)
	require.True(t, applied)

	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 10_000, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 10_000, cached.RemainQuota)
	assert.Zero(t, cached.UsedQuota)
	ledger, err := GetTaskBillingStageRecord(TaskBillingTypeTask, task.ID, base.Operation, TaskBillingStageToken)
	require.NoError(t, err)
	require.NotNil(t, ledger)
	assert.Zero(t, ledger.TokenUsedDelta)

	undone, err := UndoTaskBillingStage(tokenStage)
	require.NoError(t, err)
	require.True(t, undone)
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 8_000, reloaded.RemainQuota)
	assert.Zero(t, reloaded.UsedQuota)
	cached, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 8_000, cached.RemainQuota)
	assert.Zero(t, cached.UsedQuota)
}

func TestAggregateBaselineRejectsTaskWhoseSettlementAlreadyStarted(t *testing.T) {
	truncateTables(t)
	user := createReserveTestUser(t, 1_000)
	task := &Task{TaskID: "late-aggregate-baseline", UserId: user.Id, Quota: 100}
	insertTask(t, task)

	settlement := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Stage: TaskBillingStageFunding,
		Delta: -50, TargetQuota: 50, UserId: user.Id, BillingSource: "wallet",
	}
	applied, err := ApplyTaskBillingStage(settlement)
	require.NoError(t, err)
	require.True(t, applied)

	baseline := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "submit", Stage: TaskBillingStageAggregateBaseline,
		Delta: 100, TargetQuota: 100, RequestCountDelta: 1,
		UserId: user.Id, BillingSource: "wallet",
	}
	applied, err = ApplyTaskBillingStage(baseline)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrTaskBillingOperationPending)

	var reloadedUser User
	require.NoError(t, DB.First(&reloadedUser, user.Id).Error)
	assert.Equal(t, 1_050, reloadedUser.Quota)
	assert.Zero(t, reloadedUser.UsedQuota)
	assert.Zero(t, reloadedUser.RequestCount)
	ledger, err := GetTaskBillingStageRecord(
		TaskBillingTypeTask, task.ID, baseline.Operation, baseline.Stage,
	)
	require.NoError(t, err)
	assert.Nil(t, ledger)

	finalize := settlement
	finalize.Stage = TaskBillingStageFinalize
	applied, err = ApplyTaskBillingStage(finalize)
	require.NoError(t, err)
	require.True(t, applied)
	var reloadedTask Task
	require.NoError(t, DB.First(&reloadedTask, task.ID).Error)
	assert.Equal(t, 50, reloadedTask.Quota)
	assert.Equal(t, TaskAggregateUsageLegacyUnknown, reloadedTask.PrivateData.AggregateUsageState)
}

func TestTaskBillingFundingCreditCommitUnknownFencesCacheBeforeRetryAndUndo(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1000)
	task := &Task{
		TaskID: "ledger-ambiguous-funding-credit", UserId: user.Id, Quota: 100,
		PrivateData: TaskPrivateData{AggregateUsageState: TaskAggregateUsageAccounted},
	}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)

	stage := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Stage: TaskBillingStageFunding,
		Delta: -50, TargetQuota: 50, UserId: user.Id, BillingSource: "wallet",
	}
	commitTransportErr := errors.New("funding credit commit result unavailable")
	recheckErr := errors.New("funding credit ledger recheck unavailable")
	oldRun := runTaskBillingTransaction
	oldResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, recheckErr
	}
	restore := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldResolve
	}
	t.Cleanup(restore)

	applied, err := ApplyTaskBillingStage(stage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	assert.Equal(t, 1050, getUserQuotaFromDB(t, user.Id))
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
	assert.True(t, server.Exists(getTaskBillingUserQuotaFenceKey(user.Id)))

	restore()
	applied, err = ApplyTaskBillingStage(stage)
	require.NoError(t, err)
	assert.False(t, applied)
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1050, cached.Quota)

	undone, err := UndoTaskBillingStage(stage)
	require.NoError(t, err)
	assert.True(t, undone)
	assert.Equal(t, 1000, getUserQuotaFromDB(t, user.Id))
	cached, err = GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1000, cached.Quota)
}

func TestTaskBillingTokenCreditCommitUnknownFencesExactUsedDelta(t *testing.T) {
	truncateTables(t)
	server := useTaskLedgerRedis(t)
	user := createReserveTestUser(t, 1000)
	token := createReserveTestToken(t, 500)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"user_id": user.Id, "used_quota": 100,
	}).Error)
	task := &Task{
		TaskID: "ledger-ambiguous-token-credit", UserId: user.Id, Quota: 100,
		PrivateData: TaskPrivateData{AggregateUsageState: TaskAggregateUsageAccounted},
	}
	insertTask(t, task)
	_, err := GetUserCache(user.Id)
	require.NoError(t, err)
	_, err = GetTokenByKey(token.Key, true)
	require.NoError(t, err)

	base := TaskBillingStageParams{
		TaskType: TaskBillingTypeTask, TaskRecordId: task.ID,
		Operation: "settle:50", Delta: -50, TargetQuota: 50,
		UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, BillingSource: "wallet",
	}
	funding := base
	funding.Stage = TaskBillingStageFunding
	applied, err := ApplyTaskBillingStage(funding)
	require.NoError(t, err)
	require.True(t, applied)

	tokenStage := base
	tokenStage.Stage = TaskBillingStageToken
	commitTransportErr := errors.New("token credit commit result unavailable")
	recheckErr := errors.New("token credit ledger recheck unavailable")
	oldRun := runTaskBillingTransaction
	oldResolve := resolveTaskBillingApplyCommitFn
	runTaskBillingTransaction = func(fn func(*gorm.DB) error) error {
		if err := DB.Transaction(fn); err != nil {
			return err
		}
		return commitTransportErr
	}
	resolveTaskBillingApplyCommitFn = func(TaskBillingStageParams, string) (bool, bool, error) {
		return false, false, recheckErr
	}
	restore := func() {
		runTaskBillingTransaction = oldRun
		resolveTaskBillingApplyCommitFn = oldResolve
	}
	t.Cleanup(restore)

	applied, err = ApplyTaskBillingStage(tokenStage)
	assert.False(t, applied)
	assert.ErrorIs(t, err, ErrTaskBillingCommitUncertain)
	reloaded := getTokenFromDB(t, token.Id)
	assert.Equal(t, 550, reloaded.RemainQuota)
	assert.Equal(t, 50, reloaded.UsedQuota)
	assert.False(t, server.Exists(getTokenCacheKey(token.Key)))
	assert.True(t, server.Exists(getTaskBillingTokenQuotaFenceKey(token.Key)))

	restore()
	applied, err = ApplyTaskBillingStage(tokenStage)
	require.NoError(t, err)
	assert.False(t, applied)
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 550, cached.RemainQuota)
	assert.Equal(t, 50, cached.UsedQuota)

	ledger, err := GetTaskBillingStageRecord(tokenStage.TaskType, tokenStage.TaskRecordId, tokenStage.Operation, tokenStage.Stage)
	require.NoError(t, err)
	require.NotNil(t, ledger)
	assert.Equal(t, -50, ledger.TokenUsedDelta)
	undone, err := UndoTaskBillingStage(tokenStage)
	require.NoError(t, err)
	assert.True(t, undone)
	reloaded = getTokenFromDB(t, token.Id)
	assert.Equal(t, 500, reloaded.RemainQuota)
	assert.Equal(t, 100, reloaded.UsedQuota)
	cached, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, 500, cached.RemainQuota)
	assert.Equal(t, 100, cached.UsedQuota)
}
