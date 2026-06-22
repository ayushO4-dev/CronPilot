// Package update checks GitHub Releases for a newer CronPilot build, downloads
// the binary for the running platform (verifying its SHA-256 against the
// release's SHA256SUMS), swaps it in for the current executable, and restarts.
//
// The source repository defaults to ayushO4-dev/CronPilot and can be overridden
// with CRONPILOT_UPDATE_REPO ("owner/name").
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func repoSlug() string {
	if v := strings.TrimSpace(os.Getenv("CRONPILOT_UPDATE_REPO")); v != "" {
		return v
	}
	return "ayushO4-dev/CronPilot"
}

// assetName is the release asset for the running platform, e.g.
// "cronpilotd-linux-arm64".
func assetName() string {
	return fmt.Sprintf("cronpilotd-%s-%s", runtime.GOOS, runtime.GOARCH)
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Body       string    `json:"body"`
	HTMLURL    string    `json:"html_url"`
	Assets     []ghAsset `json:"assets"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
}

// Release is a resolved, downloadable release for this platform.
type Release struct {
	Version string
	Tag     string
	Notes   string
	URL     string
	asset   ghAsset
	sumURL  string
}

// CheckResult is the JSON-facing comparison returned to the UI.
type CheckResult struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes,omitempty"`
	URL       string `json:"url,omitempty"`
	Asset     string `json:"asset,omitempty"`
}

func httpClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

// Latest fetches and resolves the latest (non-draft) release for this platform.
func Latest(ctx context.Context) (*Release, error) {
	url := "https://api.github.com/repos/" + repoSlug() + "/releases/latest"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}
	var gr ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&gr); err != nil {
		return nil, err
	}
	rel := &Release{
		Version: normalizeVersion(gr.TagName),
		Tag:     gr.TagName,
		Notes:   gr.Body,
		URL:     gr.HTMLURL,
	}
	want := assetName()
	for _, a := range gr.Assets {
		switch a.Name {
		case want:
			rel.asset = a
		case "SHA256SUMS":
			rel.sumURL = a.URL
		}
	}
	if rel.asset.URL == "" {
		return nil, fmt.Errorf("release %s has no asset %q", gr.TagName, want)
	}
	return rel, nil
}

// Check compares the latest release against the current version.
func Check(ctx context.Context, current string) (*CheckResult, *Release, error) {
	rel, err := Latest(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &CheckResult{
		Current:   normalizeVersion(current),
		Latest:    rel.Version,
		Available: semverLess(normalizeVersion(current), rel.Version),
		Notes:     rel.Notes,
		URL:       rel.URL,
		Asset:     rel.asset.Name,
	}, rel, nil
}

// ProgressFn receives (downloaded, total) byte counts during Download. total is
// -1 when the server does not report a Content-Length.
type ProgressFn func(downloaded, total int64)

// Download fetches the release binary into dir, verifies its SHA-256 against the
// release SHA256SUMS, marks it executable, and returns the temp file path.
func (r *Release) Download(ctx context.Context, dir string, progress ProgressFn) (string, error) {
	sum, err := r.fetchChecksum(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch checksum: %w", err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.asset.URL, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: %s", resp.Status)
	}

	tmp, err := os.CreateTemp(dir, ".cronpilotd-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp (is %s writable?): %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpPath) }

	h := sha256.New()
	var done int64
	total := resp.ContentLength
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				cleanup()
				return "", werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			cleanup()
			return "", rerr
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, sum) {
		os.Remove(tmpPath)
		return "", fmt.Errorf("checksum mismatch (got %s, want %s)", got, sum)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func (r *Release) fetchChecksum(ctx context.Context) (string, error) {
	if r.sumURL == "" {
		return "", errors.New("release has no SHA256SUMS asset")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.sumURL, nil)
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == r.asset.Name {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", r.asset.Name)
}

// TargetDir is the directory of the running executable — the temp download must
// live there so the final replace is an atomic, same-filesystem rename.
func TargetDir() (string, error) {
	exe, err := resolvedExe()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Apply replaces the current executable with the downloaded binary.
func Apply(newPath string) error {
	exe, err := resolvedExe()
	if err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		return fmt.Errorf("replace %s (is its directory writable by the service user?): %w", exe, err)
	}
	return nil
}

// Restart relaunches the daemon. Under systemd (Restart=always) it exits and is
// relaunched with the new binary; otherwise it re-execs in place.
func Restart() {
	if os.Getenv("INVOCATION_ID") != "" { // launched by systemd
		os.Exit(0)
	}
	exe, err := resolvedExe()
	if err != nil {
		os.Exit(0)
	}
	_ = syscall.Exec(exe, os.Args, os.Environ())
	os.Exit(0) // only reached if exec failed
}

func resolvedExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r, nil
	}
	return exe, nil
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
}

// semverLess reports whether a < b for dotted numeric versions (e.g. 0.2.0).
func semverLess(a, b string) bool {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parseVer(v string) [3]int {
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		num := parts[i]
		if j := strings.IndexAny(num, "-+"); j >= 0 { // drop pre-release/build suffix
			num = num[:j]
		}
		out[i], _ = strconv.Atoi(num)
	}
	return out
}
