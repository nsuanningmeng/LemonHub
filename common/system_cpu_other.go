//go:build !linux

package common

import "time"

type containerCPUUsage struct{}

func newContainerCPUUsage() *containerCPUUsage { return &containerCPUUsage{} }

func (*containerCPUUsage) cpuSaturation(time.Time) (float64, bool) { return 0, false }
