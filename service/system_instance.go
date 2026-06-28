package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const systemInstanceReportInterval = 30 * time.Second

var systemInstanceReporterOnce sync.Once

type SystemInstanceInfo struct {
	SchemaVersion int                       `json:"schema_version"`
	Node          common.NodeIdentity       `json:"node"`
	Role          SystemInstanceRoleInfo    `json:"role"`
	Runtime       SystemInstanceRuntimeInfo `json:"runtime"`
	Host          SystemInstanceHostInfo    `json:"host"`
	Resources     SystemInstanceResources   `json:"resources,omitempty"`
	Extra         map[string]any            `json:"extra,omitempty"`
}

type SystemInstanceRoleInfo struct {
	IsMaster bool `json:"is_master"`
}

type SystemInstanceRuntimeInfo struct {
	Version   string `json:"version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	StartedAt int64  `json:"started_at"`
}

type SystemInstanceHostInfo struct {
	Hostname string `json:"hostname"`
}

type SystemInstanceResources struct {
	CPU     SystemInstanceResourceUsage  `json:"cpu"`
	Memory  SystemInstanceResourceUsage  `json:"memory"`
	Storage SystemInstanceStorageMetrics `json:"storage"`
}

type SystemInstanceResourceUsage struct {
	UsagePercent float64 `json:"usage_percent"`
}

type SystemInstanceStorageMetrics struct {
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

func StartSystemInstanceReporter() {
	systemInstanceReporterOnce.Do(func() {
		gopool.Go(func() {
			reportAndPruneSystemInstances()

			ticker := time.NewTicker(systemInstanceReportInterval)
			defer ticker.Stop()
			for range ticker.C {
				reportAndPruneSystemInstances()
			}
		})
	})
}

func ReportCurrentSystemInstance() error {
	identity := common.GetNodeIdentity()
	hostname, hostnameErr := os.Hostname()
	if strings.TrimSpace(identity.Name) == "" {
		if hostnameErr != nil || strings.TrimSpace(hostname) == "" {
			return fmt.Errorf("system instance node name is empty")
		}
		identity.Name = hostname
		identity.Source = common.NodeNameSourceHostname
		identity.ManuallyConfigured = false
		identity.ShouldConfigureManually = true
	}
	systemStatus := common.GetSystemStatus()
	diskInfo := common.GetDiskSpaceInfo()
	info := SystemInstanceInfo{
		SchemaVersion: 1,
		Node:          identity,
		Role: SystemInstanceRoleInfo{
			IsMaster: common.IsMasterNode,
		},
		Runtime: SystemInstanceRuntimeInfo{
			Version:   common.Version,
			GOOS:      runtime.GOOS,
			GOARCH:    runtime.GOARCH,
			StartedAt: common.StartTime,
		},
		Host: SystemInstanceHostInfo{
			Hostname: hostname,
		},
		Resources: SystemInstanceResources{
			CPU: SystemInstanceResourceUsage{
				UsagePercent: systemStatus.CPUUsage,
			},
			Memory: SystemInstanceResourceUsage{
				UsagePercent: systemStatus.MemoryUsage,
			},
			Storage: SystemInstanceStorageMetrics{
				TotalBytes:  diskInfo.Total,
				UsedBytes:   diskInfo.Used,
				FreeBytes:   diskInfo.Free,
				UsedPercent: diskInfo.UsedPercent,
			},
		},
	}
	return model.UpsertSystemInstance(identity.Name, info, common.StartTime, common.GetTimestamp())
}

// reportAndPruneSystemInstances refreshes this node's heartbeat and then, on the
// master node, prunes peers that have been silent past the retention window.
// Pruning is skipped whenever this cycle's own heartbeat did not land, which
// guarantees the current node never deletes its own row: a successful report just
// wrote last_seen_at = now, microseconds before the cutoff is computed, so the
// row cannot satisfy last_seen_at < now-retention for any positive retention.
func reportAndPruneSystemInstances() {
	if err := ReportCurrentSystemInstance(); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system instance report failed; skipping prune this cycle: %v", err))
		return
	}
	pruneStaleSystemInstancesWithLog()
}

// pruneStaleSystemInstancesWithLog removes instances whose last heartbeat is
// older than the configured retention window. It runs only on the master node so
// a single writer reclaims rows left behind by replaced containers (e.g. an
// image update changes the auto hostname used as node_name). It must be called
// only after a successful self-report in the same cycle (see
// reportAndPruneSystemInstances), so the current node's own fresh row is never a
// prune candidate. last_seen_at is written with each node's local clock, so keep
// node clocks roughly in sync (NTP); a node that cannot persist a heartbeat for
// longer than the retention window is pruned and simply re-registers on its next
// successful report.
func pruneStaleSystemInstancesWithLog() {
	if !common.IsMasterNode {
		return
	}
	retention := common.SystemInstanceRetentionSeconds
	if retention <= 0 {
		return
	}
	cutoff := common.GetTimestamp() - retention
	removed, err := model.PruneStaleSystemInstances(cutoff)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system instance prune failed: %v", err))
		return
	}
	if removed > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("system instance prune removed %d stale instance(s) (retention=%ds)", removed, retention))
	}
}
