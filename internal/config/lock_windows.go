//go:build windows

package config

import "os"

// withLock on Windows is best-effort: we serialise within a single process
// but do not currently hold a kernel-level file lock across processes.
// Realistic blast radius is small (concurrent twoctl runs racing on
// config.yaml lose the earlier update) and the proper fix is to use
// LockFileEx via golang.org/x/sys/windows in a follow-up.
//
// Documented in INF-1318 (Han-9, Windows-specific residual).
func withLock(path string, fn func() error) error {
	if err := os.MkdirAll(dirOf(path+".lock"), 0o700); err != nil {
		return err
	}
	return fn()
}
