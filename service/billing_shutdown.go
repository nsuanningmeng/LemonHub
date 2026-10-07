package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Registration happens before dispatch, so a refund queued by a handler is
// visible even when the worker pool has not started running it yet.
var billingRefunds sync.WaitGroup
var billingRefundFailuresMu sync.Mutex
var billingRefundFailures int
var billingRefundLastError error
var ErrBillingRefundsFailed = errors.New("asynchronous billing refund failed")

func recordBillingRefundFailure(err error) {
	if err == nil {
		return
	}
	billingRefundFailuresMu.Lock()
	defer billingRefundFailuresMu.Unlock()
	billingRefundFailures++
	billingRefundLastError = err
}

// WaitBillingRefunds drains refunds after HTTP handlers and background billing
// producers have stopped. It must run before flushing batch accounting or
// closing the database. It does not retry non-idempotent wallet credits.
func WaitBillingRefunds(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		billingRefunds.Wait()
		close(done)
	}()
	select {
	case <-done:
		billingRefundFailuresMu.Lock()
		defer billingRefundFailuresMu.Unlock()
		if billingRefundFailures > 0 {
			return errors.Join(ErrBillingRefundsFailed, fmt.Errorf("%d asynchronous refunds require reconciliation; latest failure: %w", billingRefundFailures, billingRefundLastError))
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
