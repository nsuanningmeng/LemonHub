//go:build linux

package common

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerCPUUsesProcessHierarchy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		files     map[string]string
		usageFile string
		next      string
		cpus      float64
		want      float64
	}{
		{
			name: "v2 parent quota accounts for siblings",
			files: map[string]string{
				"/proc/self/cgroup":               "0::/pod/leaf",
				"/proc/self/mountinfo":            "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw",
				"/sys/fs/cgroup/pod/leaf/cpu.max": "max 100000",
				"/sys/fs/cgroup/pod/cpu.max":      "200000 100000",
				"/sys/fs/cgroup/pod/cpu.stat":     "usage_usec 1000000",
			}, usageFile: "/sys/fs/cgroup/pod/cpu.stat", next: "usage_usec 11000000", cpus: 4, want: 100,
		},
		{
			name: "parent siblings do not inherit leaf CPU affinity",
			files: map[string]string{
				"/proc/self/cgroup":               "0::/pod/leaf",
				"/proc/self/mountinfo":            "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw",
				"/sys/fs/cgroup/pod/leaf/cpu.max": "max 100000",
				"/sys/fs/cgroup/pod/cpu.max":      "800000 100000",
				"/sys/fs/cgroup/pod/cpu.stat":     "usage_usec 1000000",
			}, usageFile: "/sys/fs/cgroup/pod/cpu.stat", next: "usage_usec 21000000", cpus: 1, want: 50,
		},
		{
			name: "v2 subtree quota above affinity",
			files: map[string]string{
				"/proc/self/cgroup":    "0::/pod/leaf",
				"/proc/self/mountinfo": "29 23 0:26 /pod /cg rw - cgroup2 cgroup rw",
				"/cg/leaf/cpu.max":     "200000 100000",
				"/cg/leaf/cpu.stat":    "usage_usec 1000000",
			}, usageFile: "/cg/leaf/cpu.stat", next: "usage_usec 6000000", cpus: 1, want: 100,
		},
		{
			name: "v1 combined mount fractional quota",
			files: map[string]string{
				"/proc/self/cgroup":          "4:cpu,cpuacct:/pod/leaf",
				"/proc/self/mountinfo":       "29 23 0:26 /pod /cg rw - cgroup cgroup rw,cpu,cpuacct",
				"/cg/leaf/cpu.cfs_quota_us":  "50000",
				"/cg/leaf/cpu.cfs_period_us": "100000",
				"/cg/leaf/cpuacct.usage":     "1000000000",
			}, usageFile: "/cg/leaf/cpuacct.usage", next: "2250000000", cpus: 4, want: 50,
		},
		{
			name: "v1 separate controller mounts",
			files: map[string]string{
				"/proc/self/cgroup":            "4:cpu:/pod/leaf\n3:cpuacct:/pod/leaf",
				"/proc/self/mountinfo":         "29 23 0:26 /pod /cpu rw - cgroup cgroup rw,cpu\n30 23 0:27 / /acct rw - cgroup cgroup rw,cpuacct",
				"/cpu/leaf/cpu.cfs_quota_us":   "200000",
				"/cpu/leaf/cpu.cfs_period_us":  "100000",
				"/acct/pod/leaf/cpuacct.usage": "1000000000",
			}, usageFile: "/acct/pod/leaf/cpuacct.usage", next: "6000000000", cpus: 4, want: 50,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := &containerCPUUsage{cpus: tt.cpus, previous: make(map[string]cgroupCPUSample), readFile: func(name string) ([]byte, error) {
				if value, ok := tt.files[name]; ok {
					return []byte(value), nil
				}
				return nil, os.ErrNotExist
			}}
			now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
			_, ok := u.cpuSaturation(now)
			require.False(t, ok)
			tt.files[tt.usageFile] = tt.next
			got, ok := u.cpuSaturation(now.Add(5 * time.Second))
			require.True(t, ok)
			assert.InDelta(t, tt.want, got, 0.001)
			// Missing samples invalidate the old baseline rather than averaging
			// an unobserved interval into an apparently fresh measurement.
			delete(tt.files, tt.usageFile)
			_, ok = u.cpuSaturation(now.Add(10 * time.Second))
			require.False(t, ok)
			tt.files[tt.usageFile] = tt.next
			_, ok = u.cpuSaturation(now.Add(15 * time.Second))
			assert.False(t, ok)
		})
	}
}

func TestCgroupMemoryWorkingSetPreservesAncestorPressure(t *testing.T) {
	files := memoryCgroupFixture(true, "100", "95", "1000", "950")
	files["/proc/self/cgroup"] = "0::/pod/leaf"
	files["/proc/self/mountinfo"] = "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw"
	files["/sys/fs/cgroup/pod/leaf/memory.stat"] = "inactive_file 80"
	readFile := func(name string) ([]byte, error) {
		if value, ok := files[name]; ok {
			return []byte(value), nil
		}
		return nil, os.ErrNotExist
	}
	assert.Equal(t, float64(95), cgroupMemoryUsage(37, readFile), "missing parent stat keeps known pressure")
	files["/sys/fs/cgroup/pod/memory.stat"] = "inactive_file 850"
	assert.Equal(t, float64(15), cgroupMemoryUsage(37, readFile), "cache is excluded at both levels")
	files["/sys/fs/cgroup/pod/leaf/memory.stat"] = "inactive_file 101"
	assert.Equal(t, float64(10), cgroupMemoryUsage(37, readFile), "cache snapshot cannot underflow")
}

func TestContainerCPUDifferentV1MembershipDoesNotReadOtherWorkload(t *testing.T) {
	files := map[string]string{
		"/proc/self/cgroup":               "4:cpu:/pod/leaf\n3:cpuacct:/accounting/leaf",
		"/proc/self/mountinfo":            "29 23 0:26 / /cpu rw - cgroup cgroup rw,cpu\n30 23 0:27 / /acct rw - cgroup cgroup rw,cpuacct",
		"/cpu/pod/leaf/cpu.cfs_quota_us":  "100000",
		"/cpu/pod/leaf/cpu.cfs_period_us": "100000",
		"/acct/pod/leaf/cpuacct.usage":    "1000000000",
	}
	u := &containerCPUUsage{cpus: 1, previous: make(map[string]cgroupCPUSample), readFile: func(name string) ([]byte, error) {
		if value, ok := files[name]; ok {
			return []byte(value), nil
		}
		return nil, os.ErrNotExist
	}}
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	u.cpuSaturation(now)
	files["/acct/pod/leaf/cpuacct.usage"] = "6000000000"
	_, ok := u.cpuSaturation(now.Add(5 * time.Second))
	assert.False(t, ok, "unknown controller pairing must use host fallback")
}
