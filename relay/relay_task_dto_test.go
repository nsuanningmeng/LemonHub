package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TaskModel2Dto feeds every user-facing task endpoint (/api/task/self, suno
// fetch, generic video fetch). It must never expose the mapped upstream model:
// Properties.UpstreamModelName is blanked and occurrences of the mapped model
// inside the raw upstream Data are rewritten to the requested model.
func TestTaskModel2DtoHidesMappedUpstreamModel(t *testing.T) {
	task := &model.Task{
		TaskID: "task_abc",
		Properties: model.Properties{
			Input:             "a cat",
			OriginModelName:   "veo-3.1",
			UpstreamModelName: "veo-3.0-fast-generate-001",
		},
		Data: []byte(`{"name":"models/veo-3.0-fast-generate-001/operations/xyz","model":"veo-3.0-fast-generate-001"}`),
	}

	d := TaskModel2Dto(task)

	props, ok := d.Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, props.UpstreamModelName)
	assert.Equal(t, "veo-3.1", props.OriginModelName)
	assert.Equal(t, "a cat", props.Input)
	assert.JSONEq(t, `{"name":"task_abc","model":"veo-3.1"}`, string(d.Data))

	// The task model itself must stay untouched (it may be persisted later).
	assert.Equal(t, "veo-3.0-fast-generate-001", task.Properties.UpstreamModelName)
	assert.Contains(t, string(task.Data), "veo-3.0-fast-generate-001")
}

// Vertex operation names embed the GCP project id and region; the "name"
// field must be replaced with the public task id for non-admin output
// regardless of model mapping (admin queries re-attach raw Data).
func TestTaskModel2DtoRedactsOperationName(t *testing.T) {
	task := &model.Task{
		TaskID: "task_abc",
		Properties: model.Properties{
			OriginModelName:   "veo-3.1",
			UpstreamModelName: "veo-3.1",
		},
		Data: []byte(`{"name":"projects/my-gcp-project/locations/us-central1/publishers/google/models/veo-3.1/operations/xyz","done":false}`),
	}

	d := TaskModel2Dto(task)

	assert.JSONEq(t, `{"name":"task_abc","done":false}`, string(d.Data))
	// The task model itself must stay untouched.
	assert.Contains(t, string(task.Data), "my-gcp-project")
}

func TestTaskModel2DtoKeepsNonOperationName(t *testing.T) {
	task := &model.Task{
		TaskID: "task_abc",
		Properties: model.Properties{
			OriginModelName:   "suno_music",
			UpstreamModelName: "suno_music",
		},
		Data: []byte(`{"name":"My Song","status":"complete"}`),
	}

	d := TaskModel2Dto(task)

	assert.JSONEq(t, `{"name":"My Song","status":"complete"}`, string(d.Data))
}
