package common

import "math"

// SumTokenCountsForStatistics adds observed token counts in the int64 statistics
// domain. Invalid provider counts must not wrap or reduce usage. This is not a
// quota conversion: neither the per-request charge cap nor wallet limits apply.
func SumTokenCountsForStatistics(counts ...int64) int64 {
	var total int64
	for _, count := range counts {
		if count < 0 {
			SysError("negative token count in usage statistics; clamping to zero")
			continue
		}
		if count > math.MaxInt64-total {
			SysError("token count overflow in usage statistics; saturating to int64 maximum")
			return math.MaxInt64
		}
		total += count
	}
	return total
}
