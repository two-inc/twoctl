package updater

import "testing"

func TestIsNewerExpanded(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3-rc.1", "v1.2.3", true}, // semver: rc < release
		{"v2.0.0", "v1.99.99", false},
		{"", "v1.0.0", true},
		{"dev", "", false}, // empty latest cannot be newer
		{"junk", "v1.0.0", true},
		{"v1.0.0", "junk", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
