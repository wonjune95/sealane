package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// throttle counts failed local sign-ins per key ("u:<user>", "ip:<addr>").
// After throttleMax failures inside throttleWindow the key is blocked for
// throttleBlock from the last failure. In-memory, per pod.
type throttle struct {
	mu    sync.Mutex
	fails map[string][]time.Time
	now   func() time.Time
}

const (
	throttleMax    = 5
	throttleWindow = 10 * time.Minute
	throttleBlock  = time.Minute
)

func newThrottle() *throttle { return &throttle{fails: map[string][]time.Time{}, now: time.Now} }

// blocked returns the remaining block time; 0 = allowed.
func (t *throttle) blocked(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	ts := t.trim(key)
	if len(ts) < throttleMax {
		return 0
	}
	if until := ts[len(ts)-1].Add(throttleBlock); t.now().Before(until) {
		return until.Sub(t.now())
	}
	return 0
}

func (t *throttle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fails[key] = append(t.trim(key), t.now())
	// ponytail: entries are trimmed on access only; a flood of distinct keys grows the map until each key's window passes.
}

func (t *throttle) reset(key string) { t.mu.Lock(); delete(t.fails, key); t.mu.Unlock() }

// trim drops failures older than the window; caller holds the lock.
func (t *throttle) trim(key string) []time.Time {
	cut := t.now().Add(-throttleWindow)
	ts := t.fails[key]
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(t.fails, key)
	} else {
		t.fails[key] = ts
	}
	return ts
}

// clientIP is the first X-Forwarded-For entry, else the RemoteAddr host.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
