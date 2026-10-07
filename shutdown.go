package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// http.Server.Shutdown does not wait for hijacked connections. Track the whole
// handler, including its settlement/refund defers. Hijacked sockets are closed
// on shutdown so an idle WebSocket cannot indefinitely block final accounting.
type drainingHandler struct {
	next    http.Handler
	mu      sync.Mutex
	active  map[*drainingRequest]struct{}
	stopped bool
	done    chan struct{}
}

type drainingRequest struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   net.Conn // protected by the handler mutex
}

func newDrainingHandler(next http.Handler) *drainingHandler {
	return &drainingHandler{next: next, active: make(map[*drainingRequest]struct{}), done: make(chan struct{})}
}

func (h *drainingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		w.Header().Set("Connection", "close")
		http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	request := &drainingRequest{ctx: ctx, cancel: cancel}
	h.active[request] = struct{}{}
	h.mu.Unlock()
	defer func() {
		cancel()
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.active, request)
		if h.stopped && len(h.active) == 0 {
			close(h.done)
		}
	}()
	h.next.ServeHTTP(&drainingResponseWriter{ResponseWriter: w, owner: h, request: request}, r.WithContext(ctx))
}

func (h *drainingHandler) stop() {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.stopped = true
	if len(h.active) == 0 {
		close(h.done)
	}
	var hijacked []*drainingRequest
	for request := range h.active {
		if request.conn != nil {
			hijacked = append(hijacked, request)
		}
	}
	h.mu.Unlock()
	for _, request := range hijacked {
		request.cancel()
		_ = request.conn.Close()
	}
}

// Unwrap preserves ResponseController support (deadlines/full duplex); Flush
// and Hijack preserve the interfaces used directly by Gin and WebSocket/SSE.
type drainingResponseWriter struct {
	http.ResponseWriter
	owner   *drainingHandler
	request *drainingRequest
}

func (w *drainingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *drainingResponseWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *drainingResponseWriter) CloseNotify() <-chan bool {
	if notifier, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	notify := make(chan bool, 1)
	go func() {
		<-w.request.ctx.Done()
		notify <- true
	}()
	return notify
}

func (w *drainingResponseWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

func (w *drainingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buffer, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.owner.mu.Lock()
	w.request.conn = conn
	stopped := w.owner.stopped
	w.owner.mu.Unlock()
	if stopped {
		// Shutdown can race a handshake already inside the handler.
		w.request.cancel()
		_ = conn.Close()
	}
	return conn, buffer, nil
}

func (h *drainingHandler) wait(ctx context.Context) error {
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// shutdownAccounting keeps the database open until every accounting producer
// is quiescent, asynchronous refunds finish, and the final batch is persisted.
// A timeout or persistence failure must not be reported as a clean shutdown.
func shutdownAccounting(ctx context.Context, server *http.Server, requests *drainingHandler) error {
	requests.stop()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("drain HTTP requests: %w", err)
	}
	if err := requests.wait(ctx); err != nil {
		return fmt.Errorf("drain active handlers (including WebSocket): %w", err)
	}
	if err := service.StopSystemTaskRunner(ctx); err != nil {
		return fmt.Errorf("stop background billing tasks: %w", err)
	}
	refundErr := service.WaitBillingRefunds(ctx)
	if refundErr != nil && !errors.Is(refundErr, service.ErrBillingRefundsFailed) {
		return fmt.Errorf("drain asynchronous refunds: %w", refundErr)
	}
	if err := model.StopBatchUpdater(ctx); err != nil {
		return errors.Join(refundErr, fmt.Errorf("stop periodic accounting flush: %w", err))
	}
	flushErr := model.FlushBatchUpdates(ctx)
	if flushErr != nil {
		flushErr = fmt.Errorf("persist final accounting batch: %w", flushErr)
	}
	// Even a completed refund that failed must not discard unrelated healthy
	// accounting or dashboard data. Preserve it before reporting non-clean exit.
	var dashboardErr error
	if common.DataExportEnabled {
		if err := model.SaveQuotaDataCacheContext(ctx); err != nil {
			dashboardErr = fmt.Errorf("persist final dashboard batch: %w", err)
		}
	}
	return errors.Join(refundErr, flushErr, dashboardErr)
}
