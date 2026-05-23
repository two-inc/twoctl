package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const (
	owner = "two-inc"
	repo  = "twoctl-cli"
	// checkInterval throttles the network call made by AutoCheck so that
	// every invocation of twoctl doesn't hit the GitHub API.
	checkInterval = 24 * time.Hour
)

// Release is the subset of the GitHub release payload that we care about.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestRelease fetches the latest release tag from GitHub.
func LatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no releases published yet")
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// IsNewer reports whether `latest` is a strictly newer semver tag than
// `current`. Both are expected in `vX.Y.Z` (or `X.Y.Z`) form. A non-semver
// `current` (the "dev" build) is always treated as older so devs see the
// prompt. A non-semver `latest` is treated as "not newer" - we will not
// nag users to upgrade to a tag we can't reason about.
func IsNewer(current, latest string) bool {
	l := canonicalSemver(latest)
	if l == "" {
		// We will not nag users to upgrade to a tag we can't parse.
		return false
	}
	if current == "" || current == "dev" {
		return true
	}
	c := canonicalSemver(current)
	if c == "" {
		// Unparseable current (custom build, weird tag) - treat as
		// "behind any real release" so the user sees the prompt.
		return true
	}
	return semver.Compare(l, c) > 0
}

// canonicalSemver normalises an input tag for x/mod/semver, which requires a
// leading `v`. Returns "" if the input is not a valid semver string at all.
func canonicalSemver(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return semver.Canonical(v)
}

// AutoCheck runs at most once per checkInterval. It returns the latest
// release if it is newer than `current` and the user has not skipped it,
// otherwise nil. Network failures are silent - we never want to block a
// command on the update check.
func AutoCheck(ctx context.Context, current string) (*Release, error) {
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	if !state.AutoCheckEnabled() {
		return nil, nil
	}
	now := time.Now().UTC()
	if now.Before(state.NextCheckAt) {
		// Throttled: return the cached release if it's still upgrade-worthy.
		if state.CachedRelease != nil &&
			IsNewer(current, state.CachedRelease.TagName) &&
			!state.IsSkipped(state.CachedRelease.TagName) {
			return state.CachedRelease, nil
		}
		return nil, nil
	}

	// Pre-claim the next slot so concurrent invocations don't all race to
	// hit GitHub. Saves before the network call - this is the key fix for
	// the "two CLIs started within the same second both hit GitHub" race.
	state.NextCheckAt = now.Add(checkInterval)
	if err := SaveState(state); err != nil {
		return nil, err
	}

	rel, err := LatestRelease(ctx)
	if err != nil {
		// Pre-claim already persisted; on network failure we just wait
		// the full interval before trying again.
		return nil, nil
	}
	state.LatestKnownVersion = rel.TagName
	state.CachedRelease = rel
	if err := SaveState(state); err != nil {
		return nil, err
	}
	if !IsNewer(current, rel.TagName) || state.IsSkipped(rel.TagName) {
		return nil, nil
	}
	return rel, nil
}

// maxDownloadBytes caps the asset + checksum downloads. 100 MiB is well
// over any realistic twoctl release size and well under any merchant's
// available RAM.
const maxDownloadBytes = 100 << 20

// checksumsAssetName is the canonical goreleaser checksum file name.
const checksumsAssetName = "checksums.txt"

// Apply downloads the release asset matching the host platform, verifies its
// SHA-256 against the release's checksums.txt, and replaces the running
// binary in place. The caller's next invocation of twoctl will run the new
// version.
func Apply(ctx context.Context, rel *Release) error {
	asset, err := selectAsset(rel)
	if err != nil {
		return err
	}
	wantSum, err := fetchChecksum(ctx, rel, asset.Name)
	if err != nil {
		return fmt.Errorf("verifying release: %w", err)
	}
	archive, err := downloadAsset(ctx, asset)
	if err != nil {
		return err
	}
	gotSum := sha256.Sum256(archive)
	if hex.EncodeToString(gotSum[:]) != wantSum {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s - aborting upgrade",
			asset.Name, wantSum, hex.EncodeToString(gotSum[:]))
	}
	body, err := extractBinary(asset.Name, bytes.NewReader(archive))
	if err != nil {
		return err
	}
	return selfupdate.Apply(body, selfupdate.Options{})
}

func downloadAsset(ctx context.Context, asset *Asset) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download %s: HTTP %d", asset.Name, resp.StatusCode)
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > maxDownloadBytes {
		return nil, fmt.Errorf("release asset %s exceeds %d byte download cap", asset.Name, maxDownloadBytes)
	}
	return buf, nil
}

// fetchChecksum retrieves the goreleaser-published checksums.txt from the
// release and returns the SHA-256 hex for the named asset. Without it we
// can't verify integrity, so a missing checksums.txt is a hard error.
func fetchChecksum(ctx context.Context, rel *Release, assetName string) (string, error) {
	var sumsAsset *Asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == checksumsAssetName {
			sumsAsset = &rel.Assets[i]
			break
		}
	}
	if sumsAsset == nil {
		return "", fmt.Errorf("release %s has no %s; refusing to upgrade without integrity check",
			rel.TagName, checksumsAssetName)
	}
	body, err := downloadAsset(ctx, sumsAsset)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s does not list %s", checksumsAssetName, assetName)
}

// assetNameRe matches goreleaser archive names strictly:
//
//	<project>_<version>_<os>_<arch>.<ext>
//
// Using a strict regex avoids the substring-match confusion where an asset
// like `twoctl_evil_linux_x86_64-debug.tar.gz` could win over the real one.
var assetNameRe = regexp.MustCompile(`^[A-Za-z0-9]+_[^_]+_([a-z0-9]+)_([a-z0-9_]+)\.(tar\.gz|tgz|zip)$`)

// selectAsset picks the archive matching GOOS/GOARCH. goreleaser names
// archives like `twoctl_0.1.0_darwin_arm64.tar.gz`. Matching is strict (full
// regex, exact OS + arch tokens) to prevent substring-collision attacks.
func selectAsset(rel *Release) (*Asset, error) {
	wantOS := runtime.GOOS
	wantArch := runtime.GOARCH
	if wantArch == "amd64" {
		wantArch = "x86_64"
	}
	for i := range rel.Assets {
		m := assetNameRe.FindStringSubmatch(rel.Assets[i].Name)
		if m == nil {
			continue
		}
		if m[1] == wantOS && m[2] == wantArch {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("no release asset for %s/%s in %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
}

// extractBinary unwraps the goreleaser archive (tar.gz on unix, zip on
// windows) and returns a reader positioned at the twoctl binary.
func extractBinary(name string, r io.Reader) (io.Reader, error) {
	switch {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return extractFromTarGz(r)
	case strings.HasSuffix(name, ".zip"):
		buf, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return extractFromZip(buf)
	}
	return r, nil
}

// PromptResponse is the user's answer to the upgrade prompt.
type PromptResponse int

// Possible PromptResponse values.
const (
	PromptUpgrade PromptResponse = iota
	PromptDecline
	PromptSkip
)

// AskAndApply prompts the user, then either upgrades, declines, or marks the
// version as skipped according to the response. Returns the chosen response.
func AskAndApply(ctx context.Context, rel *Release, currentVersion string, in io.Reader, out io.Writer) (PromptResponse, error) {
	fmt.Fprintf(out, "\ntwoctl %s is available (you are on %s).\n", rel.TagName, displayVersion(currentVersion))
	fmt.Fprintf(out, "  [y] upgrade now\n  [n] not now\n  [s] skip this version (won't ask again until next release)\n")
	fmt.Fprint(out, "choice [y/n/s] (default n): ")
	buf := make([]byte, 8)
	n, _ := in.Read(buf)
	resp := strings.ToLower(strings.TrimSpace(string(buf[:n])))
	switch resp {
	case "y", "yes":
		fmt.Fprintln(out, "downloading...")
		if err := Apply(ctx, rel); err != nil {
			fmt.Fprintf(out,
				"upgrade failed: %v\nyour existing binary is still in place; if you see odd behaviour, reinstall from https://github.com/two-inc/twoctl-cli/releases\n",
				err)
			return PromptUpgrade, err
		}
		fmt.Fprintf(out, "upgraded to %s. re-run your command to use the new binary.\n", rel.TagName)
		return PromptUpgrade, nil
	case "s", "skip":
		state, err := LoadState()
		if err != nil {
			return PromptSkip, err
		}
		state.AddSkip(rel.TagName)
		if err := SaveState(state); err != nil {
			return PromptSkip, err
		}
		fmt.Fprintf(out, "skipped %s. run `twoctl upgrade --reset-skips` to undo.\n", rel.TagName)
		return PromptSkip, nil
	default:
		return PromptDecline, nil
	}
}

func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "a development build"
	}
	return v
}
