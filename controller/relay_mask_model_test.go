package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newMaskContext(t *testing.T, originModel, upstreamModel string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(constant.ContextKeyOriginalModel), originModel)
	c.Set(string(constant.ContextKeyUpstreamModelName), upstreamModel)
	return c
}

// User-visible error text must show the requested model instead of the mapped
// upstream model, without corrupting longer model tokens that merely contain
// the mapped name as a substring.
func TestMaskMappedModelName(t *testing.T) {
	cases := []struct {
		name     string
		origin   string
		upstream string
		message  string
		want     string
	}{
		{
			name:     "plain occurrence replaced",
			origin:   "gpt-4o",
			upstream: "deepseek-chat",
			message:  "The model `deepseek-chat` does not exist",
			want:     "The model `gpt-4o` does not exist",
		},
		{
			name:     "substring of longer token untouched",
			origin:   "gpt-5-mini",
			upstream: "gpt-5",
			message:  "Rate limit reached for gpt-5-mini in organization org-x",
			want:     "Rate limit reached for gpt-5-mini in organization org-x",
		},
		{
			name:     "exact token replaced next to punctuation",
			origin:   "gpt-5-mini",
			upstream: "gpt-5",
			message:  "The model gpt-5 does not exist.",
			want:     "The model gpt-5-mini does not exist.",
		},
		{
			name:     "path-delimited occurrence replaced",
			origin:   "veo-3.1",
			upstream: "veo-3.0-fast-generate-001",
			message:  "operation models/veo-3.0-fast-generate-001/operations/xyz failed",
			want:     "operation models/veo-3.1/operations/xyz failed",
		},
		{
			name:     "no upstream key leaves message unchanged",
			origin:   "gpt-4o",
			upstream: "",
			message:  "The model `deepseek-chat` does not exist",
			want:     "The model `deepseek-chat` does not exist",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newMaskContext(t, tc.origin, tc.upstream)
			assert.Equal(t, tc.want, maskMappedModelName(c, tc.message))
		})
	}
}
