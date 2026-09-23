package common

import (
	"container/list"
	"sync"
	"time"
)

// rateLimitRequest stores one accepted request without allocating for the limit.
type rateLimitRequest struct {
	next      *rateLimitRequest
	timestamp int64
}

type rateLimitQueue struct {
	head   *rateLimitRequest
	tail   *rateLimitRequest
	length int
}

func (q *rateLimitQueue) append(timestamp int64) {
	request := &rateLimitRequest{timestamp: timestamp}
	if q.tail == nil {
		q.head = request
	} else {
		q.tail.next = request
	}
	q.tail = request
	q.length++
}

func (q *rateLimitQueue) removeExpired(now, duration int64) {
	for q.head != nil && now-q.head.timestamp >= duration {
		expired := q.head
		q.head = expired.next
		expired.next = nil
		q.length--
	}
	if q.head == nil {
		q.tail = nil
	}
}

type rateLimitEntry struct {
	key        string
	lastActive time.Time
	duration   int64
	element    *list.Element
	requests   rateLimitQueue
}

// InMemoryRateLimiter uses sliding windows and evicts idle keys. Reservations
// live separately so eviction cannot forget a request that is still running.
type InMemoryRateLimiter struct {
	store              map[string]*rateLimitEntry
	lru                *list.List
	reservations       map[string]int
	mutex              sync.Mutex
	expirationDuration time.Duration
	now                func() time.Time
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.store != nil {
		return
	}
	l.store = make(map[string]*rateLimitEntry)
	l.lru = list.New()
	l.reservations = make(map[string]int)
	l.expirationDuration = expirationDuration
	if l.now == nil {
		l.now = time.Now
	}
	if expirationDuration > 0 {
		go l.clearExpiredItems(time.NewTicker(expirationDuration).C)
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems(ticks <-chan time.Time) {
	for now := range ticks {
		l.deleteExpiredEntries(now)
	}
}

func (l *InMemoryRateLimiter) deleteExpiredEntries(now time.Time) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.expirationDuration <= 0 {
		return
	}
	for oldest := l.lru.Back(); oldest != nil; {
		entry := oldest.Value.(*rateLimitEntry)
		if now.Sub(entry.lastActive) < l.expirationDuration {
			break
		}
		previous := oldest.Prev()
		entry.requests.removeExpired(now.Unix(), entry.duration)
		// A model window can exceed the shared limiter's idle-cleanup interval.
		// Never discard successful requests that still belong to that window.
		if entry.requests.length == 0 {
			delete(l.store, entry.key)
			l.lru.Remove(oldest)
		}
		oldest = previous
	}
}

// touchEntry must be called with the mutex held.
func (l *InMemoryRateLimiter) touchEntry(key string, now time.Time, duration int64) *rateLimitEntry {
	entry, ok := l.store[key]
	if !ok {
		entry = &rateLimitEntry{key: key}
		entry.element = l.lru.PushFront(entry)
		l.store[key] = entry
	} else {
		l.lru.MoveToFront(entry.element)
	}
	entry.lastActive = now
	entry.duration = duration
	return entry
}

// Request records an accepted request immediately. Duration is in seconds.
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	if maxRequestNum <= 0 {
		return false
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := l.now()
	entry := l.touchEntry(key, now, duration)
	entry.requests.removeExpired(now.Unix(), duration)
	if entry.requests.length >= maxRequestNum {
		return false
	}
	entry.requests.append(now.Unix())
	return true
}

// RateLimitReservation counts an in-flight request until its outcome is known.
type RateLimitReservation struct {
	limiter  *InMemoryRateLimiter
	key      string
	duration int64
	once     sync.Once
}

// Reserve admits only when accepted requests plus in-flight requests fit the
// limit. A nil reservation means the key is saturated or its limit is invalid.
func (l *InMemoryRateLimiter) Reserve(key string, maxRequests int, duration int64) *RateLimitReservation {
	if maxRequests <= 0 {
		return nil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := l.now()
	entry := l.touchEntry(key, now, duration)
	entry.requests.removeExpired(now.Unix(), duration)
	if entry.requests.length >= maxRequests || l.reservations[key] >= maxRequests-entry.requests.length {
		return nil
	}
	l.reservations[key]++
	return &RateLimitReservation{limiter: l, key: key, duration: duration}
}

// Complete is idempotent. Success records at completion time; failure only frees
// the reservation. Callers should defer Complete(false) for panic/early returns.
func (r *RateLimitReservation) Complete(success bool) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		l := r.limiter
		l.mutex.Lock()
		defer l.mutex.Unlock()
		l.reservations[r.key]--
		if l.reservations[r.key] == 0 {
			delete(l.reservations, r.key)
		}
		if success {
			now := l.now()
			entry := l.touchEntry(r.key, now, r.duration)
			entry.requests.removeExpired(now.Unix(), r.duration)
			entry.requests.append(now.Unix())
		}
	})
}
