package cli

import "testing"

func TestDeriveResource(t *testing.T) {
	cases := []struct {
		stem, path, want string
	}{
		{"order", "/v1/order/{id}", "order"},
		{"snake_case_thing", "/x", "snake-case-thing"},
		{"by", "/v1/things", "things"}, // filler → fallback to path
		{"", "/v1/foo/bar", "foo-bar"}, // empty → fallback to path
		{"__weird__", "/x", "weird"},   // collapsed underscores
	}
	for _, c := range cases {
		if got := deriveResource(c.stem, c.path); got != c.want {
			t.Errorf("deriveResource(%q, %q) = %q, want %q", c.stem, c.path, got, c.want)
		}
	}
}
