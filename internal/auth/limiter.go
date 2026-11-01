package auth

import (
	"crypto/sha256"
	"sync"
	"time"
)

// limiter bounds attempts and memory. It uses the connection peer, never
// client-supplied forwarded headers. Exhaustion fails closed until entries expire.
type limiter struct {
	mu      sync.Mutex
	entries map[[32]byte]attempts
}
type attempts struct {
	count int
	reset time.Time
}

func newLimiter() *limiter { return &limiter{entries: make(map[[32]byte]attempts)} }
func (l *limiter) allow(source, account string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, value := range l.entries {
		if !now.Before(value.reset) {
			delete(l.entries, key)
		}
	}
	keys := [][32]byte{sha256.Sum256([]byte("source:" + source)), sha256.Sum256([]byte("account:" + account))}
	limits := []int{30, 5}
	for i, key := range keys {
		if l.entries[key].count >= limits[i] {
			return false
		}
	}
	newEntries := 0
	for _, key := range keys {
		if _, exists := l.entries[key]; !exists {
			newEntries++
		}
	}
	if len(l.entries)+newEntries > 10000 {
		return false
	}
	for _, key := range keys {
		entry := l.entries[key]
		if entry.reset.IsZero() {
			entry.reset = now.Add(15 * time.Minute)
		}
		entry.count++
		l.entries[key] = entry
	}
	return true
}
