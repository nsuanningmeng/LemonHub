package openai

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type speechGateBody struct {
	first, second []byte
	step          int
	release       chan struct{}
	once          sync.Once
}

func (b *speechGateBody) Read(p []byte) (int, error) {
	if b.step == 0 {
		n := copy(p, b.first)
		b.first = b.first[n:]
		if len(b.first) == 0 {
			b.step++
		}
		return n, nil
	}
	if b.step == 1 {
		<-b.release
		n := copy(p, b.second)
		b.second = b.second[n:]
		if len(b.second) == 0 {
			b.step++
			return n, io.EOF
		}
		return n, nil
	}
	return 0, io.EOF
}
func (b *speechGateBody) Close() error { b.once.Do(func() { close(b.release) }); return nil }

type speechFlushWriter struct {
	header  http.Header
	data    bytes.Buffer
	flushed chan struct{}
	once    sync.Once
}

func (w *speechFlushWriter) Header() http.Header         { return w.header }
func (w *speechFlushWriter) WriteHeader(int)             {}
func (w *speechFlushWriter) Write(p []byte) (int, error) { return w.data.Write(p) }
func (w *speechFlushWriter) Flush()                      { w.once.Do(func() { close(w.flushed) }) }
func speechFixture(w http.ResponseWriter, body io.ReadCloser) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{Request: &dto.AudioRequest{StreamFormat: "audio", ResponseFormat: "pcm"}}
	info.SetEstimatePromptTokens(1)
	return c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"audio/pcm"}}, Body: body}, info
}
func TestBinarySpeechFlushesFirstChunkBeforeReadingTerminal(t *testing.T) {
	first := bytes.Repeat([]byte{0x00, 0xff, 'd', 'a', 't', 'a', ':'}, 3428)
	first = append(first, bytes.Repeat([]byte{0}, 24000-len(first))...)
	second := bytes.Repeat([]byte{0x7f}, 24000)
	body := &speechGateBody{first: first, second: second, release: make(chan struct{})}
	defer body.Close()
	w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
	c, resp, info := speechFixture(w, body)
	finished := make(chan struct{})
	var usage *dto.Usage
	go func() { usage = OpenaiTTSHandler(c, resp, info); close(finished) }()
	select {
	case <-w.flushed:
	case <-time.After(2 * time.Second):
		body.Close()
		<-finished
		t.Fatal("first binary chunk was buffered until EOF")
	}
	body.Close()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("audio handler did not finish")
	}
	require.Equal(t, append(first, second...), w.data.Bytes())
	assert.Equal(t, "audio/pcm", w.header.Get("Content-Type"))
	assert.False(t, info.IsStream)
	assert.Equal(t, 17, usage.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, 18, usage.TotalTokens)
	assert.NotContains(t, w.data.String(), "[DONE]")
}

type speechReadBody struct {
	data   []byte
	err    error
	closed atomic.Int32
}

func (b *speechReadBody) Read(p []byte) (int, error) {
	n := copy(p, b.data)
	b.data = b.data[n:]
	if len(b.data) > 0 {
		return n, nil
	}
	return n, b.err
}
func (b *speechReadBody) Close() error { b.closed.Add(1); return nil }

type speechErrorWriter struct {
	*speechFlushWriter
	writeErr, errorFlush error
	short                bool
	cancel               context.CancelFunc
}

func (w *speechErrorWriter) Write(p []byte) (int, error) {
	if w.cancel != nil {
		w.cancel()
	}
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.speechFlushWriter.Write(p)
}

type speechOuterFlushError struct {
	gin.ResponseWriter
	target *speechErrorWriter
}

func (w *speechOuterFlushError) FlushError() error { return w.target.errorFlush }
func TestBinarySpeechObservedBytesSurviveReadWriteShortFlushAndTerminalCancel(t *testing.T) {
	sentinel := errors.New("fixture transport failure https://private.invalid/path?key=private_audio_secret")
	for _, kind := range []string{"read", "write", "short", "flush", "terminal_cancel", "partial_cancel"} {
		t.Run(kind, func(t *testing.T) {
			body := &speechReadBody{data: bytes.Repeat([]byte{1}, 48000), err: io.EOF}
			w := &speechErrorWriter{speechFlushWriter: &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}}
			switch kind {
			case "read":
				body.err = sentinel
			case "write":
				w.writeErr = sentinel
			case "short":
				w.short = true
			case "flush":
				w.errorFlush = sentinel
			}
			c, resp, info := speechFixture(w, body)
			if kind == "flush" {
				c.Writer = &speechOuterFlushError{ResponseWriter: c.Writer, target: w}
			}
			if kind == "terminal_cancel" || kind == "partial_cancel" {
				if kind == "partial_cancel" {
					body.err = nil
				}
				body.data = body.data[:24000]
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				w.cancel = cancel
			}
			usage := OpenaiTTSHandler(c, resp, info)
			assert.Equal(t, 17, usage.CompletionTokenDetails.AudioTokens)
			assert.Equal(t, int32(1), body.closed.Load())
			assert.NotContains(t, w.data.String(), "data: ")
			assert.NotContains(t, w.data.String(), "[DONE]")
			if kind == "terminal_cancel" || kind == "partial_cancel" {
				assert.True(t, info.StreamOutcome().DownstreamCancelled)
				assert.Equal(t, kind == "partial_cancel", info.IsPureDownstreamCancellation())
				assert.False(t, info.StreamOutcome().CancelledWithoutBillableOutput)
			} else {
				assert.True(t, info.StreamOutcome().UpstreamFailed)
				assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyResponseFailed))
				assert.True(t, info.StreamStatus.HasErrors())
				assert.NotContains(t, info.StreamStatus.Summary(), "private_audio_secret")
			}
		})
	}
}

type speechBlockedBody struct {
	entered, closed chan struct{}
	err             error
	once            sync.Once
}

func (b *speechBlockedBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.closed
	if b.err != nil {
		return 0, b.err
	}
	return 0, io.ErrClosedPipe
}
func (b *speechBlockedBody) Close() error { b.once.Do(func() { close(b.closed) }); return nil }
func TestBinarySpeechCancellationClosesBlockedBodyWithoutOutput(t *testing.T) {
	for _, closeErr := range []error{io.ErrClosedPipe, net.ErrClosed, io.EOF} {
		t.Run(closeErr.Error(), func(t *testing.T) {
			body := &speechBlockedBody{entered: make(chan struct{}), closed: make(chan struct{}), err: closeErr}
			w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
			c, resp, info := speechFixture(w, body)
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			finished := make(chan struct{})
			go func() { OpenaiTTSHandler(c, resp, info); close(finished) }()
			<-body.entered
			cancel()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				body.Close()
				<-finished
				t.Fatal("binary cancel did not release body")
			}
			assert.True(t, info.IsPureDownstreamCancellation())
			assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
			assert.Empty(t, w.data.Bytes())
			assert.False(t, info.StreamOutcome().UpstreamFailed)
		})
	}
}

func TestBinaryAudioSpoolBoundsAndRemovesTemporaryFile(t *testing.T) {
	var spool binaryAudioSpool
	defer spool.close()
	spool.append(bytes.Repeat([]byte{1}, binaryAudioMemoryLimit+1))
	require.NotNil(t, spool.file)
	name := spool.file.Name()
	assert.LessOrEqual(t, spool.memory.Len(), binaryAudioMemoryLimit)
	spool.append(bytes.Repeat([]byte{2}, binaryAudioSpoolLimit))
	assert.True(t, spool.disabled)
	assert.Nil(t, spool.file)
	_, err := os.Stat(name)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = spool.reader()
	require.Error(t, err)
}
func TestBinarySpeechWAVPreservesExistingDurationAndFallback(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			data := bytes.Repeat([]byte{0}, 48000)
			if valid {
				header := make([]byte, 44)
				copy(header, "RIFF")
				binary.LittleEndian.PutUint32(header[4:], uint32(len(data)+36))
				copy(header[8:], "WAVEfmt ")
				binary.LittleEndian.PutUint32(header[16:], 16)
				binary.LittleEndian.PutUint16(header[20:], 1)
				binary.LittleEndian.PutUint16(header[22:], 1)
				binary.LittleEndian.PutUint32(header[24:], 24000)
				binary.LittleEndian.PutUint32(header[28:], 48000)
				binary.LittleEndian.PutUint16(header[32:], 2)
				binary.LittleEndian.PutUint16(header[34:], 16)
				copy(header[36:], "data")
				binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
				data = append(header, data...)
			}
			body := &speechReadBody{data: append([]byte(nil), data...), err: io.EOF}
			w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
			c, resp, info := speechFixture(w, body)
			info.Request.(*dto.AudioRequest).ResponseFormat = "wav"
			resp.Header.Set("Content-Type", "text/html")
			usage := OpenaiTTSHandler(c, resp, info)
			assert.Equal(t, data, w.data.Bytes())
			assert.Equal(t, "audio/wav", w.header.Get("Content-Type"))
			if valid {
				assert.Equal(t, 17, usage.CompletionTokenDetails.AudioTokens)
			} else {
				assert.Equal(t, 48, usage.CompletionTokenDetails.AudioTokens)
			}
		})
	}
}

type speechConstantReader byte

func (r speechConstantReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

type speechCountingWriter struct {
	header  http.Header
	count   int64
	invalid bool
}

func (w *speechCountingWriter) Header() http.Header { return w.header }
func (w *speechCountingWriter) WriteHeader(int)     {}
func (w *speechCountingWriter) Write(p []byte) (int, error) {
	for _, v := range p {
		if v != 0x13 {
			w.invalid = true
		}
	}
	w.count += int64(len(p))
	return len(p), nil
}
func (w *speechCountingWriter) Flush() {}
func TestBinarySpeechOverSpoolLimitKeepsAllBytesAndExistingFallback(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	const size = binaryAudioSpoolLimit + 1000
	body := io.NopCloser(io.LimitReader(speechConstantReader(0x13), size))
	w := &speechCountingWriter{header: make(http.Header)}
	c, resp, info := speechFixture(w, body)
	info.Request.(*dto.AudioRequest).ResponseFormat = "aac"
	usage := OpenaiTTSHandler(c, resp, info)
	assert.Equal(t, int64(size), w.count)
	assert.False(t, w.invalid)
	assert.Equal(t, 33556, usage.CompletionTokenDetails.AudioTokens)
	files, err := os.ReadDir(temp)
	require.NoError(t, err)
	assert.Empty(t, files, "bounded duration spool must be cleaned even when over cap")
}
func TestSpeechUnspecifiedFormatKeepsNonStreamingPCMContract(t *testing.T) {
	body := &speechReadBody{data: bytes.Repeat([]byte{1}, 48000), err: io.EOF}
	w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
	c, resp, info := speechFixture(w, body)
	info.Request.(*dto.AudioRequest).StreamFormat = ""
	usage := OpenaiTTSHandler(c, resp, info)
	assert.Equal(t, 48000, w.data.Len())
	assert.Equal(t, 17, usage.CompletionTokenDetails.AudioTokens)
	assert.Equal(t, "audio/pcm", w.header.Get("Content-Type"))
	select {
	case <-w.flushed:
		t.Fatal("unspecified format unexpectedly forced chunk flush")
	default:
	}
}

func TestSpeechSSEFormatKeepsExistingEventContract(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	data := `{"type":"speech.audio.delta","audio":"AAE="}`
	body := io.NopCloser(strings.NewReader("data: " + data + "\n\ndata: [DONE]\n\n"))
	w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
	c, resp, info := speechFixture(w, body)
	info.Request.(*dto.AudioRequest).StreamFormat = "sse"
	info.IsStream = true
	OpenaiTTSHandler(c, resp, info)
	assert.Equal(t, "data: "+data+"\n\n", w.data.String())
	assert.Equal(t, "text/event-stream", w.header.Get("Content-Type"))
	assert.True(t, info.IsStream)
}

func TestSpeechSSENestedUsageAndExplicitZeroRemainAuthoritative(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	for _, tc := range []struct {
		name, data            string
		prompt, output, total int
	}{
		{"positive", `{"type":"speech.audio.done","usage":{"input_tokens":9,"output_tokens":3,"total_tokens":12,"input_tokens_details":{"text_tokens":9},"output_tokens_details":{"audio_tokens":3}},"private_extension":"preserve"}`, 9, 3, 12},
		{"zero", `{"type":"speech.audio.done","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`, 0, 0, 0},
		{"empty", `{"type":"speech.audio.delta","usage":{}}`, 100, 0, 100},
		{"null_usage", `{"type":"speech.audio.delta","usage":null}`, 100, 0, 100},
		{"null_counter", `{"type":"speech.audio.delta","usage":{"input_tokens":null,"output_tokens":null,"total_tokens":null}}`, 100, 0, 100},
		{"absent", `{"type":"speech.audio.delta","audio":"AAE="}`, 100, 0, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := io.NopCloser(strings.NewReader("data: " + tc.data + "\n\ndata: [DONE]\n\n"))
			w := &speechFlushWriter{header: make(http.Header), flushed: make(chan struct{})}
			c, resp, info := speechFixture(w, body)
			info.Request.(*dto.AudioRequest).StreamFormat = "sse"
			info.IsStream = true
			info.SetEstimatePromptTokens(100)
			usage := OpenaiTTSHandler(c, resp, info)
			assert.Equal(t, tc.prompt, usage.PromptTokens)
			assert.Equal(t, tc.output, usage.CompletionTokens)
			assert.Equal(t, tc.total, usage.TotalTokens)
			if tc.name == "positive" {
				assert.Equal(t, 9, usage.PromptTokensDetails.TextTokens)
				assert.Equal(t, 3, usage.CompletionTokenDetails.AudioTokens)
			}
			assert.Equal(t, "data: "+tc.data+"\n\n", w.data.String())
		})
	}
}
