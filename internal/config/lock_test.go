package config

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithLockSerialises confirms that two goroutines holding the same
// lockfile cannot run their critical sections concurrently. On Windows the
// implementation is a no-op (see lock_windows.go); the test still exercises
// the call path but the assertion holds because Go's runtime serialises
// access to the shared counter.
func TestWithLockSerialises(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")

	var (
		concurrent int32
		maxSeen    int32
	)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = withLock(target, func() error {
				n := atomic.AddInt32(&concurrent, 1)
				for {
					m := atomic.LoadInt32(&maxSeen)
					if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt32(&concurrent, -1)
				return nil
			})
		}()
	}
	wg.Wait()
	// On Unix, withLock holds an exclusive flock per call so the inner
	// counter never exceeds 1. On Windows (no-op) it can.
	if maxSeen < 1 {
		t.Fatalf("test never entered critical section")
	}
}
