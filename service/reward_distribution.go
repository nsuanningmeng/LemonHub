package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const rewardDistributionTaskType = "reward_distribution"

type rewardDistributionHandler struct{}

func init() {
	RegisterSystemTaskHandler(rewardDistributionHandler{})
}

func (rewardDistributionHandler) Type() string            { return rewardDistributionTaskType }
func (rewardDistributionHandler) Enabled() bool           { return model.RewardDistributionReady() }
func (rewardDistributionHandler) Interval() time.Duration { return 15 * time.Second }
func (rewardDistributionHandler) NewPayload() any         { return nil }

// WakeRewardDistribution is best-effort latency reduction. Registration and
// source work are already durable; the periodic scheduler recovers a lost wake.
func WakeRewardDistribution() {
	if _, _, err := EnqueueSystemTask(rewardDistributionTaskType, nil); err != nil {
		common.SysError("reward distribution wake failed; periodic runner will retry: " + err.Error())
	}
}

func (rewardDistributionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	total := model.RewardDistributionResult{}
	for {
		result, err := model.RunRewardDistributionPass(ctx, 100)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		total.EventsProcessed += result.EventsProcessed
		total.CampaignsProcessed += result.CampaignsProcessed
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, total); err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		if !result.More {
			if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, total, ""); err != nil {
				common.SysError("reward distribution completion failed: " + err.Error())
			}
			return
		}
		// Batches are bounded and cancellable, while a busy source queue keeps
		// draining under the same renewed lease instead of being rate-limited
		// to one batch every scheduler interval.
	}
}
