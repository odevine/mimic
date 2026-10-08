// Package releases finds the newest ui release on GitHub and the file in it that
// updates the running app. The repository also publishes engine releases, so a
// release counts only when its tag starts with ui/v
package releases

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// TagPrefix starts the tag of every ui release
const TagPrefix = "ui/v"

// Release is one published ui release
type Release struct {
	// Version is the tag without its ui/v prefix
	Version string
	Name    string
	Notes   string
	// URL is the release's page, where the installers are
	URL       string
	Published time.Time
	Assets    []Asset
}

// Asset is one file attached to a release
type Asset struct {
	Name string
	Size int64
	URL  string
}

// Client reads releases from the GitHub API
type Client struct {
	HTTP *http.Client
	// Repo is "owner/name"
	Repo string
	// BaseURL replaces https://api.github.com, for a test
	BaseURL string
}

// NewClient returns a client for repo with a short timeout
func NewClient(repo string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, Repo: repo}
}

type apiRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	HTMLURL     string     `json:"html_url"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	PublishedAt time.Time  `json:"published_at"`
	Assets      []apiAsset `json:"assets"`
}

type apiAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

// ErrNone means the repository has no published ui release
var ErrNone = errors.New("no ui release has been published")

// Latest returns the newest published ui release by version. A draft or a
// prerelease is skipped, and so is a tag that is not a version, so a release
// cut for the engine never counts
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases?per_page=50", base, c.Repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking GitHub for releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var list []apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("reading the release list: %w", err)
	}

	var best *apiRelease
	for i := range list {
		r := &list[i]
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, TagPrefix) {
			continue
		}
		if !semver.IsValid("v" + strings.TrimPrefix(r.TagName, TagPrefix)) {
			continue
		}
		if best == nil || semver.Compare("v"+strings.TrimPrefix(r.TagName, TagPrefix), "v"+strings.TrimPrefix(best.TagName, TagPrefix)) > 0 {
			best = r
		}
	}
	if best == nil {
		return nil, ErrNone
	}
	out := &Release{
		Version:   strings.TrimPrefix(best.TagName, TagPrefix),
		Name:      best.Name,
		Notes:     best.Body,
		URL:       best.HTMLURL,
		Published: best.PublishedAt,
	}
	for _, a := range best.Assets {
		out.Assets = append(out.Assets, Asset{Name: a.Name, Size: a.Size, URL: a.URL})
	}
	return out, nil
}

// Newer reports whether version a is newer than version b. A version that is
// not valid, such as the dev of an unstamped build, is older than any valid one
func Newer(a, b string) bool {
	return semver.Compare("v"+strings.TrimPrefix(a, "v"), "v"+strings.TrimPrefix(b, "v")) > 0
}

// artifacts are the files an update swaps in, one pattern per platform. The
// installers carry an -installer suffix or another extension, so these never
// match them
var artifacts = map[string]*regexp.Regexp{
	"darwin":  regexp.MustCompile(`^Mimic-.+-macos-universal\.zip$`),
	"windows": regexp.MustCompile(`^Mimic-.+-windows-%s\.zip$`),
	"linux":   regexp.MustCompile(`^Mimic-.+-linux-%s\.AppImage$`),
}

// Pick returns the asset that updates an app running on goos and goarch, or nil
// when the release has none. A release published before its files were uploaded
// has none, which is how an early check sees no update
func Pick(assets []Asset, goos, goarch string) *Asset {
	pattern, ok := artifacts[goos]
	if !ok {
		return nil
	}
	re := pattern
	if strings.Contains(pattern.String(), "%s") {
		re = regexp.MustCompile(fmt.Sprintf(pattern.String(), regexp.QuoteMeta(goarch)))
	}
	for i := range assets {
		if re.MatchString(assets[i].Name) {
			return &assets[i]
		}
	}
	return nil
}

// Find returns the named asset, or nil
func Find(assets []Asset, name string) *Asset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// ChecksumsName is the file listing the SHA-256 of every installer
const ChecksumsName = "SHA256SUMS"

// Digest returns the SHA-256 of the named asset from the release's SHA256SUMS,
// or nil when the release has no such file or no line for the asset
func (c *Client) Digest(ctx context.Context, r *Release, name string) ([]byte, error) {
	sums := Find(r.Assets, ChecksumsName)
	if sums == nil {
		return nil, nil
	}
	raw, err := c.fetch(ctx, sums.URL, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ChecksumsName, err)
	}
	return ParseChecksum(string(raw), name)
}

// ParseChecksum finds name in sha256sum output, whose lines read "<hex> name" or
// "<hex> *name". It returns nil when name is not listed
func ParseChecksum(sums, name string) ([]byte, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		d, err := hex.DecodeString(fields[0])
		if err != nil || len(d) != 32 {
			return nil, fmt.Errorf("%s lists a bad digest for %s", ChecksumsName, name)
		}
		return d, nil
	}
	return nil, nil
}

// SignatureSuffix names an asset's detached signature: the artifact's name plus
// this
const SignatureSuffix = ".sig"

// Signature returns the detached Ed25519 signature published beside the named
// asset, or nil when there is none. The file holds the 64 raw bytes or their
// base64 form
func (c *Client) Signature(ctx context.Context, r *Release, name string) ([]byte, error) {
	sig := Find(r.Assets, name+SignatureSuffix)
	if sig == nil {
		return nil, nil
	}
	raw, err := c.fetch(ctx, sig.URL, 4096)
	if err != nil {
		return nil, fmt.Errorf("reading the signature: %w", err)
	}
	if len(raw) == 64 {
		return raw, nil
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(dec) != 64 {
		return nil, errors.New("the signature file is not an Ed25519 signature")
	}
	return dec, nil
}

// Download streams an asset to w, calling progress with the bytes written so
// far and the total
func (c *Client) Download(ctx context.Context, a *Asset, w io.Writer, progress func(written, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download answered %d", resp.StatusCode)
	}
	total := a.Size
	if total == 0 {
		total = resp.ContentLength
	}
	var written int64
	buf := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (c *Client) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("answered %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
