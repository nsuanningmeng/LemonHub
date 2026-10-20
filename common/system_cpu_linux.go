//go:build linux

package common

import (
	"math"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type cgroupCPUSample struct {
	usage uint64
	at    time.Time
}

type containerCPUUsage struct {
	readFile func(string) ([]byte, error)
	cpus     float64
	previous map[string]cgroupCPUSample
}

func newContainerCPUUsage() *containerCPUUsage {
	return &containerCPUUsage{readFile: os.ReadFile, cpus: float64(runtime.NumCPU()), previous: make(map[string]cgroupCPUSample)}
}

// cpuSaturation pairs each visible quota with usage charged to that same
// cgroup. This also respects parent quotas and non-root/combined v1 mounts.
func (u *containerCPUUsage) cpuSaturation(now time.Time) (float64, bool) {
	var pressure float64
	available := false
	next := make(map[string]cgroupCPUSample)
	for _, mount := range processCgroupMounts("cpu", u.readFile) {
		for directory := mount.leaf; ; directory = path.Dir(directory) {
			quota, usage, ok := u.readCPU(mount, directory)
			if ok {
				next[directory] = cgroupCPUSample{usage: usage, at: now}
				previous, found := u.previous[directory]
				if found && now.After(previous.at) && usage >= previous.usage {
					cores := quota
					// A parent's usage includes siblings with their own affinity;
					// only the leaf can be normalized to this process's CPUs.
					if directory == mount.leaf && u.cpus > 0 {
						cores = math.Min(cores, u.cpus)
					}
					current := float64(usage-previous.usage) / now.Sub(previous.at).Seconds() / 1e6 / cores * 100
					pressure = math.Max(pressure, current)
					available = true
				}
			}
			if directory == mount.mountpoint {
				break
			}
		}
	}
	u.previous = next
	return pressure, available
}

func (u *containerCPUUsage) readCPU(mount resourceCgroupMount, directory string) (float64, uint64, bool) {
	if mount.v2 {
		limit, err := u.readFile(path.Join(directory, "cpu.max"))
		if err != nil {
			return 0, 0, false
		}
		fields := strings.Fields(string(limit))
		if len(fields) != 2 {
			return 0, 0, false
		}
		quota, errQuota := strconv.ParseUint(fields[0], 10, 64)
		period, errPeriod := strconv.ParseUint(fields[1], 10, 64)
		stat, errStat := u.readFile(path.Join(directory, "cpu.stat"))
		usage, ok := cgroupStatUint(stat, "usage_usec")
		if errQuota != nil || errPeriod != nil || errStat != nil || quota == 0 || period == 0 || !ok {
			return 0, 0, false
		}
		return float64(quota) / float64(period), usage, true
	}
	quotaBytes, errQuota := u.readFile(path.Join(directory, "cpu.cfs_quota_us"))
	periodBytes, errPeriod := u.readFile(path.Join(directory, "cpu.cfs_period_us"))
	quota, errQuotaValue := strconv.ParseUint(strings.TrimSpace(string(quotaBytes)), 10, 64)
	period, errPeriodValue := strconv.ParseUint(strings.TrimSpace(string(periodBytes)), 10, 64)
	if errQuota != nil || errPeriod != nil || errQuotaValue != nil || errPeriodValue != nil || quota == 0 || period == 0 {
		return 0, 0, false
	}
	// cpuacct may be mounted separately. Match hierarchy paths, not mount names.
	cgroupPath := path.Join(mount.root, strings.TrimPrefix(directory, mount.mountpoint))
	cpuMembership := path.Join(mount.root, strings.TrimPrefix(mount.leaf, mount.mountpoint))
	for _, accounting := range processCgroupMounts("cpuacct", u.readFile) {
		if accounting.v2 || (accounting.root != "/" && cgroupPath != accounting.root && !strings.HasPrefix(cgroupPath, accounting.root+"/")) {
			continue
		}
		accountingMembership := path.Join(accounting.root, strings.TrimPrefix(accounting.leaf, accounting.mountpoint))
		if accountingMembership != cpuMembership {
			// Independent hierarchies need not group the same processes. A
			// similarly named directory could account for another workload.
			continue
		}
		accountingPath := path.Join(accounting.mountpoint, strings.TrimPrefix(cgroupPath, accounting.root))
		raw, err := u.readFile(path.Join(accountingPath, "cpuacct.usage"))
		usage, errValue := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err == nil && errValue == nil {
			return float64(quota) / float64(period), usage / 1000, true
		}
	}
	return 0, 0, false
}
