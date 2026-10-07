package common

import (
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
)

type eventErrorWriter struct {
	calls int
	err   error
	short bool
}

func (w *eventErrorWriter) Header() http.Header { return make(http.Header) }
func (w *eventErrorWriter) WriteHeader(int)     {}
func (w *eventErrorWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 2 {
		if w.short {
			return 1, nil
		}
		return 0, w.err
	}
	return len(p), nil
}
func TestCustomEventPropagatesDelimiterWriteFailure(t *testing.T) {
	sentinel := errors.New("delimiter failure")
	w := &eventErrorWriter{err: sentinel}
	require.ErrorIs(t, (CustomEvent{Data: "data: hello"}).Render(w), sentinel)
	w = &eventErrorWriter{short: true}
	require.ErrorIs(t, (CustomEvent{Data: "data: hello"}).Render(w), io.ErrShortWrite)
}
