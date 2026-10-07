package common

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func TestStreamOutcomeEvidencePriorityAndAttemptReset(t *testing.T) {
	info := &RelayInfo{}
	info.MarkDownstreamCancelled()
	assert.True(t, info.IsPureDownstreamCancellation())
	info.MarkUpstreamCompleted()
	assert.False(t, info.IsPureDownstreamCancellation())
	info.MarkUpstreamFailure()
	outcome := info.StreamOutcome()
	assert.True(t, outcome.UpstreamFailed)
	assert.True(t, outcome.UpstreamCompleted)
	info.ResetStreamOutcome()
	assert.Equal(t, StreamOutcome{}, info.StreamOutcome())
	info.MarkCancelledWithoutBillableOutput()
	assert.True(t, info.IsPureDownstreamCancellation())
	assert.True(t, info.StreamOutcome().CancelledWithoutBillableOutput)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				info.MarkUpstreamFailure()
				info.MarkDownstreamCancelled()
				_ = info.StreamOutcome()
				_ = info.IsPureDownstreamCancellation()
			}
		}()
	}
	wg.Wait()
	assert.True(t, info.StreamOutcome().UpstreamFailed)
}

func TestCompletedCancellationCorrectionPreservesOtherTerminalEvidence(t *testing.T) {
	for _, reason := range []StreamEndReason{StreamEndReasonTimeout, StreamEndReasonScannerErr, StreamEndReasonHandlerStop, StreamEndReasonEOF, StreamEndReasonDone} {
		status := NewStreamStatus()
		status.SetEndReason(reason, nil)
		status.CorrectCompletedCancellationAfterJoin()
		assert.Equal(t, reason, status.EndReason)
	}
}

func TestStreamOutcomeOtherTerminalAndLocallyClassifiedFailureStatus(t *testing.T) {
	info := &RelayInfo{}
	info.MarkDownstreamCancelled()
	info.MarkOtherUpstreamTerminal()
	assert.False(t, info.IsPureDownstreamCancellation())
	assert.False(t, info.StreamOutcome().UpstreamCompleted)
	assert.False(t, info.StreamOutcome().UpstreamFailed)
	info.MarkUpstreamFailureStatus(503)
	assert.Equal(t, 503, info.StreamOutcome().UpstreamFailureStatus)
	info.MarkUpstreamFailureStatus(200)
	assert.Equal(t, 200, info.StreamOutcome().UpstreamFailureStatus, "operator-mapped HTTP status retains existing classification policy")
	info.MarkUpstreamFailureStatus(99999)
	assert.Equal(t, 200, info.StreamOutcome().UpstreamFailureStatus, "provider numeric codes are not HTTP status")
	info.ResetStreamOutcome()
	info.MarkUpstreamFailure()
	assert.Zero(t, info.StreamOutcome().UpstreamFailureStatus, "unknown failure status must not be guessed")
	assert.False(t, info.StreamOutcome().OtherUpstreamTerminal)
}
