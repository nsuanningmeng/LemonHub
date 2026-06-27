package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAttachmentIds protects the per-message attachment binding guard:
// dedup, drop non-positive ids, and enforce the cap so a single message cannot
// bind an unbounded number of uploads. maxCount == 0 means "no cap".
func TestValidateAttachmentIds(t *testing.T) {
	cases := []struct {
		name     string
		ids      []int
		maxCount int
		want     []int
		wantErr  bool
	}{
		{name: "nil input", ids: nil, maxCount: 6, want: nil},
		{name: "dedup and drop non-positive", ids: []int{1, 1, 2, -3, 0}, maxCount: 6, want: []int{1, 2}},
		{name: "exceeds cap", ids: []int{1, 2, 3, 4, 5, 6, 7}, maxCount: 6, wantErr: true},
		{name: "zero cap means unlimited", ids: []int{1, 2}, maxCount: 0, want: []int{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateAttachmentIds(tc.ids, tc.maxCount)
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
