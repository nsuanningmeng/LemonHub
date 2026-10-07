//go:build linux

package common

import (
	"math"
	"os"
	"path"
	"strconv"
	"strings"
)

type memoryCgroupMount struct {
	root       string
	mountpoint string
	leaf       string
	v2         bool
}

func memoryUsagePercent(hostUsedPercent float64) float64 {
	return cgroupMemoryUsage(hostUsedPercent, os.ReadFile)
}

// cgroupMemoryUsage uses the current process's memory hierarchy, not a fixed
// Docker path. Every visible ancestor retains its own limit and charged usage.
func cgroupMemoryUsage(hostUsedPercent float64, readFile func(string) ([]byte, error)) float64 {
	cgroups, err := readFile("/proc/self/cgroup")
	if err != nil {
		return hostUsedPercent
	}
	mountinfo, err := readFile("/proc/self/mountinfo")
	if err != nil {
		return hostUsedPercent
	}

	var v1Path, v2Path string
	for _, line := range strings.Split(string(cgroups), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		if fields[0] == "0" && fields[1] == "" {
			v2Path = fields[2]
		} else if hasCgroupController(fields[1], "memory") {
			v1Path = fields[2]
		}
	}
	// On hybrid hosts an explicitly assigned v1 memory controller owns the
	// memory accounting, even if the process also belongs to a v2 hierarchy.
	processPath, v2 := v1Path, false
	if processPath == "" {
		processPath, v2 = v2Path, true
	}
	if !validCgroupPath(processPath) {
		return hostUsedPercent
	}

	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	var mounts []memoryCgroupMount
	for _, line := range strings.Split(string(mountinfo), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 6 || len(right) < 3 {
			continue
		}
		if v2 {
			if right[0] != "cgroup2" {
				continue
			}
		} else if right[0] != "cgroup" || !hasCgroupController(right[2], "memory") {
			continue
		}
		root, mountpoint := unescape.Replace(left[3]), unescape.Replace(left[4])
		if !validCgroupPath(root) || !validCgroupPath(mountpoint) {
			continue
		}
		if root != "/" && processPath != root && !strings.HasPrefix(processPath, root+"/") {
			continue
		}
		leaf := path.Join(mountpoint, strings.TrimPrefix(processPath, root))
		mounts = append(mounts, memoryCgroupMount{root: root, mountpoint: mountpoint, leaf: leaf, v2: v2})
	}
	if len(mounts) == 0 {
		return hostUsedPercent
	}
	var pressure float64
	constrained := false
	for _, mount := range mounts {
		if current, ok := mount.memoryPressure(readFile); ok {
			pressure = math.Max(pressure, current)
			constrained = true
		}
	}
	if constrained {
		return pressure
	}
	return hostUsedPercent
}

func hasCgroupController(controllers, name string) bool {
	for _, controller := range strings.Split(controllers, ",") {
		if controller == name {
			return true
		}
	}
	return false
}

func validCgroupPath(value string) bool {
	// Namespace roots such as /.. cannot be mapped into /proc/self/cgroup's
	// coordinates safely. Never clean them into an apparently valid root.
	return strings.HasPrefix(value, "/") && path.Clean(value) == value
}

// memoryPressure pairs each applicable limit with THAT cgroup's usage. Using a
// parent's limit with only the leaf's usage would hide pressure from siblings.
func (mount memoryCgroupMount) memoryPressure(readFile func(string) ([]byte, error)) (float64, bool) {
	var pressure float64
	constrained := false
	for directory := mount.leaf; ; directory = path.Dir(directory) {
		if current, ok := mount.memoryPressureAt(directory, readFile); ok {
			pressure = math.Max(pressure, current)
			constrained = true
		}
		// An unreadable level does not invalidate a complete reading elsewhere.
		// In particular, an unknown parent must not erase a leaf's known 95%.
		if directory == mount.mountpoint {
			return pressure, constrained
		}
	}
}

func (mount memoryCgroupMount) memoryPressureAt(directory string, readFile func(string) ([]byte, error)) (float64, bool) {
	limitFile, usageFile := "memory.limit_in_bytes", "memory.usage_in_bytes"
	if mount.v2 {
		limitFile, usageFile = "memory.max", "memory.current"
	} else if directory != mount.leaf {
		hierarchy, err := readFile(path.Join(directory, "memory.use_hierarchy"))
		if err != nil || strings.TrimSpace(string(hierarchy)) != "1" {
			return 0, false
		}
	}
	limitBytes, err := readFile(path.Join(directory, limitFile))
	if err != nil {
		// The true v2 hierarchy root has no memory.max. Namespace roots with
		// finite limits have this file and are sampled like any other level.
		return 0, false
	}
	limitText := strings.TrimSpace(string(limitBytes))
	if mount.v2 && limitText == "max" {
		return 0, false
	}
	limit, err := strconv.ParseUint(limitText, 10, 64)
	if err != nil {
		return 0, false
	}
	// v1's default unlimited value is LONG_MAX rounded down to a page.
	if !mount.v2 && limit >= uint64(math.MaxInt64)&^uint64(os.Getpagesize()-1) {
		return 0, false
	}
	usageBytes, err := readFile(path.Join(directory, usageFile))
	if err != nil {
		return 0, false
	}
	usage, err := strconv.ParseUint(strings.TrimSpace(string(usageBytes)), 10, 64)
	if err != nil {
		return 0, false
	}
	if limit == 0 {
		return 100, true // A zero hard limit leaves no memory headroom.
	}
	return math.Min(float64(usage)/float64(limit)*100, 100), true
}
