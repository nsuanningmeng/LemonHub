package common

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSumTokenCountsForStatistics(t *testing.T) {
	for _, tt := range []struct {
		name   string
		counts []int64
		want   int64
	}{
		{"empty", nil, 0},
		{"inclusive input", []int64{2, 589733, 1088}, 590823},
		{"negative components cannot cancel real usage", []int64{-100, 7, -1}, 7},
		{"above the billing quota domain", []int64{math.MaxInt32, math.MaxInt32}, 2 * int64(math.MaxInt32)},
		{"exact int64 limit", []int64{math.MaxInt64 - 1, 1}, math.MaxInt64},
		{"overflow saturates", []int64{math.MaxInt64, 1}, math.MaxInt64},
	} {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, SumTokenCountsForStatistics(tt.counts...)) })
	}
}
