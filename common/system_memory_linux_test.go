//go:build linux

package common

import (
	"errors"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func memoryCgroupFixture(v2 bool, leafLimit, leafUsage, parentLimit, parentUsage string) map[string]string {
	limitFile, usageFile := "memory.limit_in_bytes", "memory.usage_in_bytes"
	if v2 {
		limitFile, usageFile = "memory.max", "memory.current"
	}
	return map[string]string{
		"/sys/fs/cgroup/pod/leaf/" + limitFile: leafLimit,
		"/sys/fs/cgroup/pod/leaf/" + usageFile: leafUsage,
		"/sys/fs/cgroup/pod/" + limitFile:      parentLimit,
		"/sys/fs/cgroup/pod/" + usageFile:      parentUsage,
	}
}

func TestCgroupMemoryUsageUsesCurrentProcessAndAncestorPressure(t *testing.T) {
	const hostPressure = 37.5
	const v2Mount = "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw"
	const v1Mount = "29 23 0:26 / /sys/fs/cgroup rw - cgroup cgroup rw,memory,rdma"
	v1Unlimited := strconv.FormatUint(uint64(math.MaxInt64)&^uint64(os.Getpagesize()-1), 10)
	tests := []struct {
		name      string
		cgroup    string
		mountinfo string
		files     map[string]string
		modify    func(map[string]string)
		want      float64
	}{
		{
			name: "v2 container charged memory", files: memoryCgroupFixture(true, "100", "95", "max", "1000"), want: 95,
		},
		{
			name: "unlimited leaf constrained by parent including siblings", files: memoryCgroupFixture(true, "max", "10", "1000", "950"), want: 95,
		},
		{
			name: "parent pressure cannot use leaf usage", files: memoryCgroupFixture(true, "1000", "100", "1000", "950"), want: 95,
		},
		{
			name: "leaf pressure exceeds parent", files: memoryCgroupFixture(true, "100", "96", "1000", "500"), want: 96,
		},
		{
			name: "all unlimited uses host", files: memoryCgroupFixture(true, "max", "10", "max", "100"), want: hostPressure,
		},
		{
			name: "zero hard limit has no headroom", files: memoryCgroupFixture(true, "0", "0", "max", "100"), want: 100,
		},
		{
			name: "over limit clamps", files: memoryCgroupFixture(true, "100", "110", "max", "100"), want: 100,
		},
		{
			name: "uint64 counters avoid overflow", files: memoryCgroupFixture(true, "18446744073709551615", "9223372036854775808", "max", "100"), want: 50,
		},
		{
			name: "namespace root is a constrained container", cgroup: "0::/", files: map[string]string{
				"/sys/fs/cgroup/memory.max": "100", "/sys/fs/cgroup/memory.current": "95",
			}, want: 95,
		},
		{
			name: "subtree mount maps current process", mountinfo: "29 23 0:26 /pod /sys/fs/cgroup rw - cgroup2 cgroup rw",
			files: map[string]string{
				"/sys/fs/cgroup/leaf/memory.max": "1000", "/sys/fs/cgroup/leaf/memory.current": "10",
				"/sys/fs/cgroup/memory.max": "1000", "/sys/fs/cgroup/memory.current": "950",
			}, want: 95,
		},
		{
			name: "unverifiable namespace mount root falls back", cgroup: "0::/", mountinfo: "29 23 0:26 /.. /sys/fs/cgroup rw - cgroup2 cgroup rw",
			files: map[string]string{"/sys/fs/cgroup/memory.max": "100", "/sys/fs/cgroup/memory.current": "95"}, want: hostPressure,
		},
		{
			name: "mount root must match full path segment", cgroup: "0::/pods/leaf", mountinfo: "29 23 0:26 /pod /sys/fs/cgroup rw - cgroup2 cgroup rw",
			files: memoryCgroupFixture(true, "100", "95", "max", "100"), want: hostPressure,
		},
		{
			name: "colon in current process path", cgroup: "0::/pod/leaf:01", files: memoryCgroupFixture(true, "100", "95", "max", "100"),
			modify: func(files map[string]string) {
				files["/sys/fs/cgroup/pod/leaf:01/memory.max"] = files["/sys/fs/cgroup/pod/leaf/memory.max"]
				files["/sys/fs/cgroup/pod/leaf:01/memory.current"] = files["/sys/fs/cgroup/pod/leaf/memory.current"]
			}, want: 95,
		},
		{
			name: "mount root and mountpoint escapes", cgroup: "0::/pod space/leaf",
			mountinfo: `29 23 0:26 /pod\040space /sys/fs/cgroup\040memory rw - cgroup2 cgroup rw`,
			files: map[string]string{
				"/sys/fs/cgroup memory/leaf/memory.max": "100", "/sys/fs/cgroup memory/leaf/memory.current": "95",
				"/sys/fs/cgroup memory/memory.max": "max",
			}, want: 95,
		},
		{
			name: "broad mount wins over first leaf bind", mountinfo: "28 23 0:26 /pod/leaf /leaf rw - cgroup2 cgroup rw\n" + v2Mount,
			files: memoryCgroupFixture(true, "1000", "10", "1000", "950"), modify: func(files map[string]string) {
				files["/leaf/memory.max"], files["/leaf/memory.current"] = "1000", "10"
			}, want: 95,
		},
		{
			name: "unreadable parent retains known leaf pressure", mountinfo: "28 23 0:26 /pod/leaf /leaf rw - cgroup2 cgroup rw\n" + v2Mount,
			files: memoryCgroupFixture(true, "100", "95", "broken", "950"), modify: func(files map[string]string) {
				files["/leaf/memory.max"], files["/leaf/memory.current"] = "1000", "10"
			}, want: 95,
		},
		{
			name: "v1 co-mounted controller and hierarchical parents", cgroup: "7:memory,rdma:/pod/leaf", mountinfo: v1Mount,
			files: memoryCgroupFixture(false, "1000", "10", "1000", "950"), modify: func(files map[string]string) {
				files["/sys/fs/cgroup/pod/memory.use_hierarchy"] = "1"
				files["/sys/fs/cgroup/memory.use_hierarchy"] = "0"
			}, want: 95,
		},
		{
			name: "v1 parent with hierarchy disabled does not constrain descendants", cgroup: "7:memory:/pod/leaf", mountinfo: v1Mount,
			files: memoryCgroupFixture(false, "100", "20", "1000", "950"), modify: func(files map[string]string) {
				files["/sys/fs/cgroup/pod/memory.use_hierarchy"] = "0"
				files["/sys/fs/cgroup/memory.use_hierarchy"] = "0"
			}, want: 20,
		},
		{
			name: "v1 unlimited sentinel falls back", cgroup: "7:memory:/pod/leaf", mountinfo: v1Mount,
			files: memoryCgroupFixture(false, v1Unlimited, "10", v1Unlimited, "950"), modify: func(files map[string]string) {
				files["/sys/fs/cgroup/pod/memory.use_hierarchy"] = "1"
				files["/sys/fs/cgroup/memory.use_hierarchy"] = "0"
			}, want: hostPressure,
		},
		{
			name: "hybrid host uses explicit v1 memory owner", cgroup: "0::/wrong\n7:memory:/pod/leaf", mountinfo: v2Mount + "\n" + v1Mount,
			files: memoryCgroupFixture(false, "100", "95", v1Unlimited, "950"), modify: func(files map[string]string) {
				files["/sys/fs/cgroup/pod/memory.use_hierarchy"] = "1"
				files["/sys/fs/cgroup/memory.use_hierarchy"] = "0"
			}, want: 95,
		},
		{
			name: "v1 controller matching is exact", cgroup: "7:notmemory:/pod/leaf", mountinfo: v1Mount,
			files: memoryCgroupFixture(false, "100", "95", "100", "95"), want: hostPressure,
		},
		{
			name: "v1 unknown ancestor hierarchy retains known leaf pressure", cgroup: "7:memory:/pod/leaf", mountinfo: v1Mount,
			files: memoryCgroupFixture(false, "100", "95", "100", "95"), want: 95,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cgroup == "" {
				tt.cgroup = "0::/pod/leaf"
			}
			if tt.mountinfo == "" {
				tt.mountinfo = v2Mount
			}
			require.NotNil(t, tt.files)
			tt.files["/proc/self/cgroup"] = tt.cgroup + "\n"
			tt.files["/proc/self/mountinfo"] = tt.mountinfo + "\n"
			if tt.modify != nil {
				tt.modify(tt.files)
			}
			readFile := func(name string) ([]byte, error) {
				value, ok := tt.files[name]
				if !ok {
					return nil, &os.PathError{Op: "read", Path: name, Err: os.ErrNotExist}
				}
				return []byte(value + "\n"), nil
			}
			assert.InDelta(t, tt.want, cgroupMemoryUsage(hostPressure, readFile), 0.00001)
		})
	}
}

func TestCgroupMemoryUsageReadFailuresPreserveKnownPressure(t *testing.T) {
	for _, tt := range []struct {
		name            string
		file            string
		value           string
		readErr         error
		unlimitedParent bool
		want            float64
	}{
		{name: "proc cgroup unavailable", file: "/proc/self/cgroup", readErr: os.ErrPermission, want: 37.5},
		{name: "mountinfo unavailable", file: "/proc/self/mountinfo", readErr: os.ErrNotExist, want: 37.5},
		{name: "current process removed", file: "/sys/fs/cgroup/pod/leaf/memory.max", readErr: os.ErrNotExist, want: 95},
		{name: "ancestor unreadable", file: "/sys/fs/cgroup/pod/memory.max", readErr: os.ErrPermission, want: 95},
		{name: "usage read fails", file: "/sys/fs/cgroup/pod/leaf/memory.current", readErr: errors.New("read failed"), want: 95},
		{name: "malformed counter", file: "/sys/fs/cgroup/pod/leaf/memory.current", value: "not-a-number", want: 95},
		{name: "negative counter", file: "/sys/fs/cgroup/pod/leaf/memory.current", value: "-1", want: 95},
		{name: "overflowing counter", file: "/sys/fs/cgroup/pod/memory.max", value: "18446744073709551616", want: 95},
		{name: "invalid process path", file: "/proc/self/cgroup", value: "0::/pod/../pod/leaf", want: 37.5},
		{name: "malformed mountinfo", file: "/proc/self/mountinfo", value: "invalid", want: 37.5},
		{name: "no readable finite pair falls back", file: "/sys/fs/cgroup/pod/leaf/memory.current", readErr: os.ErrPermission, unlimitedParent: true, want: 37.5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := memoryCgroupFixture(true, "100", "95", "1000", "950")
			if tt.unlimitedParent {
				files["/sys/fs/cgroup/pod/memory.max"] = "max"
			}
			files["/proc/self/cgroup"] = "0::/pod/leaf"
			files["/proc/self/mountinfo"] = "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw"
			readFile := func(name string) ([]byte, error) {
				if name == tt.file {
					return []byte(tt.value), tt.readErr
				}
				value, ok := files[name]
				if !ok {
					return nil, os.ErrNotExist
				}
				return []byte(value), nil
			}
			assert.Equal(t, tt.want, cgroupMemoryUsage(37.5, readFile))
		})
	}
}
