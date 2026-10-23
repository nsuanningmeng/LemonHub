package perfmetrics

import "sync"

// persistenceMu protects transfers between hot buckets and the database, not
// recording. Individual buckets keep each sample's counters together.
var persistenceMu sync.RWMutex
var persistenceRevision uint64

// queryWithHotSnapshot retries a database read if a flush moved samples while
// it was running. The fallback holds a shared lock to guarantee progress even
// when many buckets are being flushed, without blocking Record on database I/O.
func queryWithHotSnapshot[T any](readPersisted func() (T, error), include func(bucketKey) bool) (T, map[bucketKey]counters, error) {
	persistenceMu.RLock()
	revision := persistenceRevision
	hot := snapshotHotBuckets(include)
	persistenceMu.RUnlock()

	rows, err := readPersisted()
	if err != nil {
		return rows, nil, err
	}

	persistenceMu.RLock()
	defer persistenceMu.RUnlock()
	if revision == persistenceRevision {
		return rows, hot, nil
	}

	hot = snapshotHotBuckets(include)
	rows, err = readPersisted()
	return rows, hot, err
}

func snapshotHotBuckets(include func(bucketKey) bool) map[bucketKey]counters {
	hot := make(map[bucketKey]counters)
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if include(k) {
			snapshot := value.(*metricBucket).snapshot()
			if snapshot.requestCount > 0 {
				hot[k] = snapshot
			}
		}
		return true
	})
	return hot
}
