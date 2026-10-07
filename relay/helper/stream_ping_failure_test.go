package helper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	rc "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type closeObservedBody struct {
	*io.PipeReader
	closed chan struct{}
	once   sync.Once
}

func (b *closeObservedBody) Close() error {
	err := b.PipeReader.Close()
	b.once.Do(func() { close(b.closed) })
	return err
}
func TestPingFailureStopsBlockedReaderAndJoinsBeforeReturn(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled = true
	settings.PingIntervalSeconds = 1
	t.Cleanup(func() { settings.PingIntervalEnabled = oldEnabled; settings.PingIntervalSeconds = oldInterval })
	sentinel := errors.New("ping socket failure")
	w := &errorStreamWriter{header: make(http.Header)}
	c := writerContext(w)
	pr, pw := io.Pipe()
	defer pw.Close()
	body := &closeObservedBody{PipeReader: pr, closed: make(chan struct{})}
	defer body.Close()
	info := &rc.RelayInfo{ChannelMeta: &rc.ChannelMeta{}}
	finished := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: body}, info, func(data string, sr *StreamResult) {
			if err := StringData(c, data); err != nil {
				sr.Stop(err)
				return
			}
			w.err = sentinel
		})
		close(finished)
	}()
	go func() { _, _ = io.WriteString(pw, "data: {\"text\":\"ready\"}\n\n") }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		_ = body.Close()
		<-finished
		t.Fatal("ping failure waited for 30s idle timeout")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("returned before upstream body close")
	}
	require.Equal(t, rc.StreamEndReasonPingFail, info.StreamStatus.EndReason)
	require.ErrorIs(t, info.StreamStatus.EndError, sentinel)
	assert.Equal(t, 1, w.flushes)
}

type cancelOnPingDeadlineWriter struct {
	*errorStreamWriter
	cancel context.CancelFunc
	armed  bool
}

func (w *cancelOnPingDeadlineWriter) SetWriteDeadline(time.Time) error {
	if w.armed {
		w.cancel()
	}
	return nil
}
func TestPingContextCancellationRemainsDownstreamCancellation(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	settings := operation_setting.GetGeneralSetting()
	oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled = true
	settings.PingIntervalSeconds = 1
	t.Cleanup(func() { settings.PingIntervalEnabled = oldEnabled; settings.PingIntervalSeconds = oldInterval })
	base := &errorStreamWriter{header: make(http.Header)}
	w := &cancelOnPingDeadlineWriter{errorStreamWriter: base}
	c := writerContext(w)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	w.cancel = cancel
	c.Request = c.Request.WithContext(ctx)
	pr, pw := io.Pipe()
	defer pw.Close()
	body := &closeObservedBody{PipeReader: pr, closed: make(chan struct{})}
	defer body.Close()
	info := &rc.RelayInfo{ChannelMeta: &rc.ChannelMeta{}}
	finished := make(chan struct{})
	go func() {
		StreamScannerHandler(c, &http.Response{Body: body}, info, func(data string, sr *StreamResult) {
			if err := StringData(c, data); err != nil {
				sr.Stop(err)
				return
			}
			w.armed = true
		})
		close(finished)
	}()
	go func() { _, _ = io.WriteString(pw, "data: {\"text\":\"ready\"}\n\n") }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		_ = body.Close()
		<-finished
		t.Fatal("cancelled ping did not join")
	}
	require.Equal(t, rc.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	require.ErrorIs(t, info.StreamStatus.EndError, context.Canceled)
	assert.True(t, info.IsPureDownstreamCancellation())
	assert.Equal(t, 1, base.flushes)
	assert.Contains(t, string(base.body), "ready")
	assert.False(t, c.GetBool("relay_stream_write_failed"))
}

type pingFaultAndCancelWriter struct {
	*errorStreamWriter
	cancel context.CancelFunc
}

func (w *pingFaultAndCancelWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), ": PING") {
		w.cancel()
		return 0, w.err
	}
	return w.errorStreamWriter.Write(p)
}
func TestPingTransportFailureWinsLateDownstreamCancellation(t *testing.T) {
	for _, priorStatus := range []int{0, 503} {
		t.Run(fmt.Sprintf("prior_status_%d", priorStatus), func(t *testing.T) {
			oldTimeout := constant.StreamingTimeout
			constant.StreamingTimeout = 30
			t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
			settings := operation_setting.GetGeneralSetting()
			oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
			settings.PingIntervalEnabled = true
			settings.PingIntervalSeconds = 1
			t.Cleanup(func() { settings.PingIntervalEnabled = oldEnabled; settings.PingIntervalSeconds = oldInterval })
			sentinel := errors.New("actual ping transport failure")
			w := &pingFaultAndCancelWriter{errorStreamWriter: &errorStreamWriter{header: make(http.Header)}}
			c := writerContext(w)
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			w.cancel = cancel
			c.Request = c.Request.WithContext(ctx)
			pr, pw := io.Pipe()
			defer pw.Close()
			body := &closeObservedBody{PipeReader: pr, closed: make(chan struct{})}
			defer body.Close()
			info := &rc.RelayInfo{ChannelMeta: &rc.ChannelMeta{}}
			if priorStatus != 0 {
				info.MarkUpstreamFailureStatus(priorStatus)
			}
			finished := make(chan struct{})
			go func() {
				StreamScannerHandler(c, &http.Response{Body: body}, info, func(data string, sr *StreamResult) {
					if err := StringData(c, data); err != nil {
						sr.Stop(err)
						return
					}
					w.err = sentinel
				})
				close(finished)
			}()
			go func() { _, _ = io.WriteString(pw, "data: {\"text\":\"ready\"}\n\n") }()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				_ = body.Close()
				<-finished
				t.Fatal("ping transport failure did not join")
			}
			assert.True(t, info.StreamOutcome().UpstreamFailed)
			wantStatus := priorStatus
			if wantStatus == 0 {
				wantStatus = http.StatusBadGateway
			}
			assert.Equal(t, wantStatus, info.StreamOutcome().UpstreamFailureStatus)
			assert.False(t, info.IsPureDownstreamCancellation())
			assert.True(t, c.GetBool("relay_stream_write_failed"))
		})
	}
}
