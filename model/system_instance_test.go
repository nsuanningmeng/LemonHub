package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneStaleSystemInstances(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM system_instances").Error)
	truncateTables(t)

	now := common.GetTimestamp()
	// "fresh" reported just now; "stale" last reported ~3 hours ago.
	require.NoError(t, UpsertSystemInstance("fresh", map[string]any{"k": "v"}, now, now))
	require.NoError(t, UpsertSystemInstance("stale", map[string]any{"k": "v"}, now-10800, now-10800))

	// Cutoff sits between the two heartbeats: only "stale" should be removed.
	removed, err := PruneStaleSystemInstances(now - 3600)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	instances, err := ListSystemInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, "fresh", instances[0].NodeName)
}

func TestPruneStaleSystemInstancesNonPositiveCutoffIsNoop(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM system_instances").Error)
	truncateTables(t)

	now := common.GetTimestamp()
	require.NoError(t, UpsertSystemInstance("only", map[string]any{}, now-10800, now-10800))

	// A non-positive cutoff disables pruning even when stale rows exist.
	removed, err := PruneStaleSystemInstances(0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed)

	instances, err := ListSystemInstances()
	require.NoError(t, err)
	assert.Len(t, instances, 1)
}
