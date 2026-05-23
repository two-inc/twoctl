package updater

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAskAndApplyDecline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	rel := &Release{TagName: "v1.0.0"}
	in := strings.NewReader("n\n")
	var out bytes.Buffer
	resp, err := AskAndApply(context.Background(), rel, "v0.9.0", in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if resp != PromptDecline {
		t.Errorf("response = %v, want PromptDecline", resp)
	}
}

func TestAskAndApplyDeclineEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	rel := &Release{TagName: "v1.0.0"}
	in := strings.NewReader("\n") // default = decline
	var out bytes.Buffer
	resp, err := AskAndApply(context.Background(), rel, "v0.9.0", in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if resp != PromptDecline {
		t.Errorf("empty input should decline, got %v", resp)
	}
}

func TestAskAndApplySkipPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	rel := &Release{TagName: "v1.2.3"}
	in := strings.NewReader("s\n")
	var out bytes.Buffer
	resp, err := AskAndApply(context.Background(), rel, "v1.0.0", in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if resp != PromptSkip {
		t.Errorf("response = %v, want PromptSkip", resp)
	}
	s, _ := LoadState()
	if !s.IsSkipped("v1.2.3") {
		t.Error("skip not persisted to state")
	}
	if !strings.Contains(out.String(), "v1.2.3") {
		t.Errorf("output missing version: %s", out.String())
	}
}

// TestApplyDownloadFails exercises the download path against a controllable
// server returning a 5xx, verifying we surface a clear error.
func TestApplyDownloadFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rel := &Release{TagName: "v1", Assets: []Asset{
		// We need an asset name that matches the current host's GOOS/GOARCH
		// for selectAsset to find it. The simplest cross-host way is to
		// include both linux and darwin assets pointing at the same URL.
		{Name: "twoctl_1.0.0_linux_x86_64.tar.gz", BrowserDownloadURL: srv.URL},
		{Name: "twoctl_1.0.0_linux_arm64.tar.gz", BrowserDownloadURL: srv.URL},
		{Name: "twoctl_1.0.0_darwin_x86_64.tar.gz", BrowserDownloadURL: srv.URL},
		{Name: "twoctl_1.0.0_darwin_arm64.tar.gz", BrowserDownloadURL: srv.URL},
		{Name: "twoctl_1.0.0_windows_x86_64.zip", BrowserDownloadURL: srv.URL},
	}}
	err := Apply(context.Background(), rel)
	if err == nil {
		t.Error("expected error from 500 response")
	}
}
