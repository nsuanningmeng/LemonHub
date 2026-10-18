package vidu

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduMetadataCannotOverrideSelectedModelOrValidatedDimensions(t *testing.T) {
	for _, tc := range []struct {
		name, model, action, wantModel string
		duration                       int
		size                           string
		wantDuration                   int
		wantSize                       string
	}{
		{"explicit fields", "vidu1.5", constant.TaskActionTextGenerate, "vidu1.5", 4, "360p", 4, "360p"},
		{"defaults", "viduq1", constant.TaskActionTextGenerate, "viduq1", 0, "", 5, "1080p"},
		{"reference model normalization", "viduq2-pro", constant.TaskActionReferenceGenerate, "viduq2", 8, "720p", 8, "720p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{
				Duration: tc.duration, Size: tc.size, Prompt: "hello",
				Metadata: map[string]any{"model": "unselected-model", "duration": relaycommon.MaxTaskDurationSeconds + 1, "resolution": "1080p", "seed": 7},
			})
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: tc.model},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: tc.action},
			}
			body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
			require.NoError(t, err)
			wire, err := io.ReadAll(body)
			require.NoError(t, err)
			var payload requestPayload
			require.NoError(t, common.Unmarshal(wire, &payload))
			assert.Equal(t, tc.wantModel, payload.Model)
			assert.Equal(t, tc.wantDuration, payload.Duration)
			assert.Equal(t, tc.wantSize, payload.Resolution)
			assert.Equal(t, 7, payload.Seed)
		})
	}
}
