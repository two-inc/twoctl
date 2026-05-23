package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.1", "v1.0.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"v1.2.3", "v1.3.0", true},
		{"v1.2.3", "v2.0.0", true},
		{"", "v1.0.0", true},
		{"dev", "v0.0.1", true},
		{"v1.0.0-rc.1", "v1.0.0", true}, // proper semver: rc < release
		{"1.0.0", "1.0.1", true},        // no leading v
		{"junk", "alsojunk", false},     // unparseable latest -> never newer
		{"junk", "junk", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestCanonicalSemver(t *testing.T) {
	cases := map[string]string{
		"v1.2.3":       "v1.2.3",
		"1.2.3":        "v1.2.3",
		"v1.2.3-rc.1":  "v1.2.3-rc.1",
		"v1.2.3+build": "v1.2.3", // x/mod strips build metadata
		"not-a-ver":    "",
		"":             "",
	}
	for in, want := range cases {
		if got := canonicalSemver(in); got != want {
			t.Errorf("canonicalSemver(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	enabled := true
	s := &State{
		LastCheckAt:        time.Now().UTC().Truncate(time.Second),
		LatestKnownVersion: "v1.2.3",
		SkippedVersions:    []string{"v1.0.0"},
		AutoCheck:          &enabled,
	}
	if err := SaveState(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LatestKnownVersion != "v1.2.3" {
		t.Errorf("LatestKnownVersion not persisted: %v", loaded)
	}
	if !loaded.IsSkipped("v1.0.0") {
		t.Error("skip not persisted")
	}
	if !loaded.AutoCheckEnabled() {
		t.Error("AutoCheck not persisted")
	}
}

func TestStateMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || len(s.SkippedVersions) != 0 {
		t.Errorf("missing file should yield empty State, got %+v", s)
	}
	if !s.AutoCheckEnabled() {
		t.Error("default AutoCheckEnabled should be true")
	}
}

func TestStateCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	p, _ := statePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte("not json"), 0o600)
	s, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.SkippedVersions) != 0 {
		t.Error("corrupt file should be treated as empty")
	}
}

func TestAddSkipIdempotent(t *testing.T) {
	s := &State{}
	s.AddSkip("v1.0.0")
	s.AddSkip("v1.0.0")
	s.AddSkip("v1.0.1")
	if len(s.SkippedVersions) != 2 {
		t.Errorf("expected 2 unique skips, got %v", s.SkippedVersions)
	}
}

func TestAutoCheckEnabledDefault(t *testing.T) {
	s := &State{}
	if !s.AutoCheckEnabled() {
		t.Error("nil AutoCheck should mean enabled")
	}
	off := false
	s.AutoCheck = &off
	if s.AutoCheckEnabled() {
		t.Error("explicit false should mean disabled")
	}
}

func TestExtractFromTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	payload := []byte("the-binary")
	_ = tw.WriteHeader(&tar.Header{Name: "twoctl_0.1.0_linux_x86_64/twoctl", Size: int64(len(payload)), Mode: 0o755})
	_, _ = tw.Write(payload)
	tw.Close()
	gz.Close()
	r, err := extractFromTarGz(&buf)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != "the-binary" {
		t.Errorf("payload mismatch: %s", got)
	}
}

func TestExtractFromTarGzMissingBinary(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "README", Size: 0})
	tw.Close()
	gz.Close()
	if _, err := extractFromTarGz(&buf); err == nil {
		t.Error("expected error when binary missing")
	}
}

func TestExtractFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("twoctl_0.1.0_windows_x86_64/twoctl.exe")
	_, _ = f.Write([]byte("win-binary"))
	zw.Close()
	r, err := extractFromZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != "win-binary" {
		t.Errorf("payload mismatch: %s", got)
	}
}

func TestIsTwoctlBinary(t *testing.T) {
	cases := map[string]bool{
		"twoctl":          true,
		"twoctl.exe":      true,
		"dir/twoctl":      true,
		"dir/twoctl.exe":  true,
		"dir\\twoctl.exe": true,
		"README":          false,
		"twoctl.bak":      false,
	}
	for in, want := range cases {
		if got := isTwoctlBinary(in); got != want {
			t.Errorf("isTwoctlBinary(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	if displayVersion("") != "a development build" {
		t.Error()
	}
	if displayVersion("dev") != "a development build" {
		t.Error()
	}
	if displayVersion("v1.2.3") != "v1.2.3" {
		t.Error()
	}
}

// Ensure the Release struct survives a json round-trip via the GitHub shape.
func TestReleaseUnmarshal(t *testing.T) {
	in := `{"tag_name":"v0.1.0","name":"first","html_url":"x","assets":[{"name":"twoctl.tar.gz","browser_download_url":"y"}]}`
	var r Release
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	if r.TagName != "v0.1.0" || len(r.Assets) != 1 || r.Assets[0].Name != "twoctl.tar.gz" {
		t.Errorf("decoded: %+v", r)
	}
}
