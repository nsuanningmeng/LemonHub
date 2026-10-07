package helper

import (
	"context"
	rc "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type queuedCompletionBody struct {
	io.Reader
	closer  io.Closer
	release chan struct{}
	once    sync.Once
}

func (b *queuedCompletionBody) Close() error {
	b.once.Do(func() {
		close(b.release)
		if b.closer != nil {
			_ = b.closer.Close()
		}
	})
	return nil
}
func TestConfirmedCompletionCorrectsCancellationOnlyAfterCallbackJoin(t *testing.T) {
	for _, tc := range []struct {
		name              string
		complete, failure bool
		want              rc.StreamEndReason
	}{
		{"completed", true, false, rc.StreamEndReasonDone}, {"cancel_without_completion", false, false, rc.StreamEndReasonClientGone}, {"actual_upstream_failure", true, true, rc.StreamEndReasonClientGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, info := setupStreamTest(t, strings.NewReader(""))
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			body := &queuedCompletionBody{Reader: strings.NewReader("data: queued_event\n\n"), release: make(chan struct{})}
			entered := make(chan struct{})
			finished := make(chan struct{})
			// Hold the callback until main cancellation cleanup closes the body. This
			// proves transport cancellation can be recorded before queued protocol evidence.
			pr, pw := io.Pipe()
			defer pw.Close()
			body.Reader = pr
			body.closer = pr
			go func() {
				StreamScannerHandler(c, &http.Response{Body: body}, info, func(_ string, sr *StreamResult) {
					close(entered)
					<-body.release
					if tc.complete {
						info.MarkUpstreamCompleted()
						sr.Done()
					}
					if tc.failure {
						info.MarkUpstreamFailure()
						sr.Stop(context.DeadlineExceeded)
					}
				})
				close(finished)
			}()
			_, err := io.WriteString(pw, "data: queued_event\n\n")
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("callback did not enter")
			}
			cancel()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				_ = pr.Close()
				body.Close()
				<-finished
				t.Fatal("cleanup did not join callback")
			}
			assert.Equal(t, tc.want, info.StreamStatus.EndReason)
			assert.True(t, info.StreamOutcome().DownstreamCancelled)
			assert.Equal(t, !tc.complete && !tc.failure, info.IsPureDownstreamCancellation())
			if tc.complete && !tc.failure {
				assert.NoError(t, info.StreamStatus.EndError)
			}
			if tc.failure {
				assert.True(t, info.StreamOutcome().UpstreamFailed)
				assert.False(t, info.IsPureDownstreamCancellation())
			}
		})
	}
}
