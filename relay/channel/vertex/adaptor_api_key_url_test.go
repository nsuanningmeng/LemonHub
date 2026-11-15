package vertex

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLWithAPIKey(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		region    string
		stream    bool
		model     string
		baseURL   string
		want      string
	}{
		{
			name:      "project ID keeps the global region",
			projectID: "my-project",
			region:    `{"default":"global"}`,
			want:      "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/google/models/gemini-4-argon:generateContent?key=test-key",
		},
		{
			name:      "project ID keeps a regional endpoint when streaming",
			projectID: "my-project",
			region:    `{"default":"us-central1"}`,
			stream:    true,
			want:      "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-4-argon:streamGenerateContent?alt=sse&key=test-key",
		},
		{
			name:      "Claude uses the Anthropic publisher with project ID",
			projectID: "my-project",
			region:    `{"default":"us"}`,
			model:     "claude-fixture",
			stream:    true,
			want:      "https://aiplatform.us.rep.googleapis.com/v1/projects/my-project/locations/us/publishers/anthropic/models/claude-fixture:streamRawPredict?alt=sse&key=test-key",
		},
		{
			name:      "custom base keeps its host and configured project",
			projectID: " my-project ",
			region:    `{"default":"eu"}`,
			baseURL:   "https://vertex.example/v1/",
			want:      "https://vertex.example/v1/projects/my-project/locations/eu/publishers/google/models/gemini-4-argon:generateContent?key=test-key",
		},
		{
			name:   "no project ID keeps the express-mode URL",
			region: `{"default":"global"}`,
			want:   "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-4-argon:generateContent?key=test-key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.model
			if model == "" {
				model = "gemini-4-argon"
			}
			info := &relaycommon.RelayInfo{
				IsStream:        tt.stream,
				OriginModelName: model,
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiVersion:        tt.region,
					ApiKey:            "test-key",
					UpstreamModelName: model,
					ChannelBaseUrl:    tt.baseURL,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						VertexKeyType:   dto.VertexKeyTypeAPIKey,
						VertexProjectID: tt.projectID,
					},
				},
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)

			got, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVertexGoogleAPIVersionSupportsBothCredentialsAndLeavesClaudeV1(t *testing.T) {
	for _, keyType := range []dto.VertexKeyType{dto.VertexKeyTypeJSON, dto.VertexKeyTypeAPIKey} {
		for _, model := range []string{"gemini-4-argon", "imagen-4", "claude-fixture"} {
			for _, version := range []string{"", "v1", "v1beta1"} {
				info := &relaycommon.RelayInfo{OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{ApiVersion: "global", ApiKey: `{"project_id":"project"}`, UpstreamModelName: model, ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: keyType, VertexProjectID: "project", VertexAPIVersion: version}}}
				if keyType == dto.VertexKeyTypeAPIKey {
					info.ApiKey = "fixture-key"
				}
				a := &Adaptor{}
				a.Init(info)
				actual, err := a.getRequestUrl(info, model, "generateContent")
				require.NoError(t, err)
				wantVersion := "v1"
				if version == "v1beta1" && model != "claude-fixture" {
					wantVersion = version
				}
				assert.Contains(t, actual, "/"+wantVersion+"/projects/project/")
			}
		}
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{VertexAPIVersion: "../invalid"}}}
	_, err := (&Adaptor{}).getRequestUrl(info, "gemini-4-argon", "generateContent")
	require.Error(t, err)
}
