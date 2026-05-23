package updater

import (
	"runtime"
	"testing"
)

func TestSelectAssetStrict(t *testing.T) {
	// Build a release whose assets include a real match plus several
	// "almost matches" that the old Contains-based selector would have
	// incorrectly picked.
	rel := &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "twoctl_evil_linux_x86_64-debug.tar.gz"},      // bogus suffix
			{Name: "twoctl_1.0.0_darwin-linux-emulation.tar.gz"}, // contains "linux"
		},
	}
	wantArch := runtime.GOARCH
	if wantArch == "amd64" {
		wantArch = "x86_64"
	}
	// Add the real asset matching the host so the test deterministically
	// expects success on whatever the runner is.
	real := "twoctl_1.0.0_" + runtime.GOOS + "_" + wantArch + ".tar.gz"
	rel.Assets = append(rel.Assets, Asset{Name: real, BrowserDownloadURL: "https://example/x"})

	got, err := selectAsset(rel)
	if err != nil {
		t.Fatalf("selectAsset returned err: %v", err)
	}
	if got.Name != real {
		t.Errorf("selectAsset picked %q, want %q (strict regex match)", got.Name, real)
	}
}

func TestSelectAssetMismatch(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "twoctl_1.0.0_aix_powerpc64.tar.gz"},
	}}
	if _, err := selectAsset(rel); err == nil {
		t.Error("expected error when no asset matches the host")
	}
}

func TestSelectAssetRejectsLoosePatterns(t *testing.T) {
	// Names that match neither the regex nor the host triple - all
	// should be ignored even if they contain the right OS/arch substring.
	rel := &Release{Assets: []Asset{
		{Name: "evil-twoctl.tar.gz"},
		{Name: "twoctl.tar.gz"},
		{Name: "twoctl_x86_64.tar.gz"},
	}}
	if _, err := selectAsset(rel); err == nil {
		t.Error("loose names should not match")
	}
}
