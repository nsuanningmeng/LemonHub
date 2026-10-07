package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nonstreamCancelWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *nonstreamCancelWriter) Write(p []byte) (int, error) { w.cancel(); return 0, context.Canceled }
func TestOpenAINonstreamAuthoritativeZeroAndMissingUsageOnWriteCancel(t *testing.T) {
	for _, tc := range []struct {
		name, field        string
		prompt, completion int
	}{{"zero", `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, 0, 0}, {"inputzero", `,"usage":{"prompt_tokens":0,"completion_tokens":2,"total_tokens":2}`, 0, 2}, {"absent", "", 3, 0}, {"null", `,"usage":null`, 3, 0}, {"empty", `,"usage":{}`, 3, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &nonstreamCancelWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			c, info := streamFrameFixture(t, writer)
			c.Request = c.Request.WithContext(ctx)
			info.RelayFormat = types.RelayFormatOpenAI
			info.IsStream = false
			info.UpstreamModelName = "test-model"
			body := `{"model":"test-model","choices":[]` + tc.field + `}`
			usage, err := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))})
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, tc.prompt, usage.PromptTokens)
			assert.Equal(t, tc.completion, usage.CompletionTokens)
			assert.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	}
}

type nonstreamContextErrorBody struct {
	err    error
	closed bool
}

func (body *nonstreamContextErrorBody) Read([]byte) (int, error) { return 0, body.err }
func (body *nonstreamContextErrorBody) Close() error             { body.closed = true; return nil }
func TestOpenAINonstreamBodyReadCancellationPreservesOnlyActualCause(t *testing.T) {
	handlers := map[string]func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError){"chat": OpenaiHandler, "responses": OaiResponsesHandler, "responses-chat": OaiResponsesToChatHandler}
	for name, handler := range handlers {
		for _, sourceErr := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(name+sourceErr.Error(), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c, info := streamFrameFixture(t, httptest.NewRecorder())
				c.Request = c.Request.WithContext(ctx)
				body := &nonstreamContextErrorBody{err: sourceErr}
				usage, apiErr := handler(c, info, &http.Response{StatusCode: 200, Header: make(http.Header), Body: body})
				assert.Nil(t, usage)
				require.NotNil(t, apiErr)
				assert.True(t, body.closed)
				assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
				if sourceErr == context.Canceled {
					assert.ErrorIs(t, apiErr, context.Canceled)
				} else {
					assert.NotErrorIs(t, apiErr, context.Canceled)
				}
			})
		}
	}
}
