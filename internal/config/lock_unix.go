//go:build !windows

package config

import (
	"os"
	"syscall"
)

// withLock holds an exclusive advisory flock on path+".lock" for the
// duration of fn. Used to serialise read-modify-write cycles on the config
// and state files so two twoctl invocations don't lose each other's updates.
//
// Unix path: syscall.Flock with LOCK_EX (blocking). The lock is per-fd, so
// closing the file releases it; we hold a reference for the whole window.
func withLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := os.MkdirAll(dirOf(lockPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
