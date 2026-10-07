//go:build !linux

package common

func memoryUsagePercent(hostUsedPercent float64) float64 {
	return hostUsedPercent
}
