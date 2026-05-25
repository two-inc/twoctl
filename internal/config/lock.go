package config

import "path/filepath"

// dirOf is the parent directory of p. Shared by the platform-specific
// withLock implementations.
func dirOf(p string) string { return filepath.Dir(p) }
