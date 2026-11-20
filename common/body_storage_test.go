package common

import (
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReplayableBodyReaderKeepsStorageLifecycleWithCaller(t *testing.T) {
	payload := []byte(`{"model":"test-model","input":"hello"}`)
	storage, err := CreateBodyStorage(payload)
	require.NoError(t, err)
	defer storage.Close()

	body := NewReplayableBodyReader(storage)
	assert.EqualValues(t, len(payload), body.Size())

	req, err := http.NewRequest(http.MethodPost, "https://example.com", body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	_, err = req.Body.Read(make([]byte, 1))
	assert.ErrorIs(t, err, http.ErrBodyReadAfterClose)

	replayBody, err := body.NewReader()
	require.NoError(t, err, "closing the HTTP request body must not close the storage")
	replay, err := io.ReadAll(replayBody)
	require.NoError(t, err)
	require.NoError(t, replayBody.Close())
	assert.Equal(t, payload, replay)

	require.NoError(t, storage.Close())
	_, err = body.NewReader()
	require.ErrorIs(t, err, ErrStorageClosed)
}

func TestReplayableBodiesKeepIndependentCursorsAcrossAttempts(t *testing.T) {
	previous := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previous) })
	payload := []byte(`{"model":"test-model","input":"two complete attempts"}`)
	for _, disk := range []bool{false, true} {
		name := "memory"
		if disk {
			name = "disk"
		}
		t.Run(name, func(t *testing.T) {
			SetDiskCacheConfig(DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
			storage, err := CreateBodyStorage(payload)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			require.Equal(t, disk, storage.IsDisk())
			first, err := http.NewRequest(http.MethodPost, "https://example.com", NewReplayableBodyReader(storage))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, first.Body.Close()) })
			head := make([]byte, len(payload)/2)
			_, err = io.ReadFull(first.Body, head)
			require.NoError(t, err)
			// Preparing attempt 2 rewinds the original storage while attempt 1 may
			// still be writing its body in the HTTP transport.
			_, err = storage.Seek(0, io.SeekStart)
			require.NoError(t, err)
			second, err := http.NewRequest(http.MethodPost, "https://example.com", NewReplayableBodyReader(storage))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, second.Body.Close()) })
			sent, err := io.ReadAll(second.Body)
			require.NoError(t, err)
			assert.Equal(t, payload, sent)
			rest, err := io.ReadAll(first.Body)
			require.NoError(t, err)
			assert.Equal(t, payload, append(head, rest...))
		})
	}
}

func TestDiskStorageOwnerCloseKeepsReadersAliveUntilLastClose(t *testing.T) {
	previous := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previous) })
	SetDiskCacheConfig(DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
	before := GetDiskCacheStats()
	payload := []byte("complete payload after owner close")
	storage, err := newDiskStorage(payload, GetDiskCachePath())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	first, err := storage.NewReader()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	second, err := storage.NewReader()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	require.NoError(t, storage.Close())
	require.NoError(t, storage.Close())
	_, err = storage.NewReader()
	assert.ErrorIs(t, err, ErrStorageClosed)
	_, err = os.Stat(storage.filePath)
	require.NoError(t, err, "the cache file remains until open readers close")
	assert.Equal(t, before.ActiveDiskFiles+1, GetDiskCacheStats().ActiveDiskFiles)
	for _, reader := range []io.ReadCloser{first, second} {
		actual, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, payload, actual)
	}
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())
	assert.Equal(t, before.ActiveDiskFiles+1, GetDiskCacheStats().ActiveDiskFiles)
	require.NoError(t, second.Close())
	require.NoError(t, second.Close())
	_, err = os.Stat(storage.filePath)
	assert.True(t, os.IsNotExist(err), "the last reader must remove the cache file")
	assert.Equal(t, before.ActiveDiskFiles, GetDiskCacheStats().ActiveDiskFiles)
}
