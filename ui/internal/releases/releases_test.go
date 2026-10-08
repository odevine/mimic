package releases

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rel(tag string, assets ...string) map[string]any {
	var as []map[string]any
	for _, a := range assets {
		as = append(as, map[string]any{"name": a, "size": 10, "browser_download_url": "ASSET/" + a})
	}
	return map[string]any{"tag_name": tag, "name": tag, "body": "notes for " + tag, "html_url": "https://github.com/x/y/releases/tag/" + tag, "assets": as}
}

func serve(t *testing.T, releases ...map[string]any) *Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/x/y/releases"):
			raw, _ := json.Marshal(releases)
			w.Write([]byte(strings.ReplaceAll(string(raw), "ASSET/", srv.URL+"/asset/")))
		case r.URL.Path == "/asset/SHA256SUMS":
			w.Write([]byte("aa" + strings.Repeat("00", 31) + "  Mimic-1.0.0-macos-universal.zip\n" + strings.Repeat("ab", 32) + " *Mimic-1.0.0-linux-amd64.AppImage\n"))
		case strings.HasSuffix(r.URL.Path, ".sig"):
			w.Write([]byte(base64.StdEncoding.EncodeToString(make([]byte, 64))))
		case strings.HasPrefix(r.URL.Path, "/asset/"):
			w.Write([]byte("0123456789"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), Repo: "x/y", BaseURL: srv.URL}
}

func TestLatestSkipsEngineReleasesAndTakesTheHighestVersion(t *testing.T) {
	c := serve(t,
		rel("engine/v0.17.0"),
		rel("ui/v0.9.2"),
		rel("ui/v1.0.0", "Mimic-1.0.0-macos-universal.zip"),
		rel("ui/v0.10.0"),
		rel("ui/vnext"),
	)
	got, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.0.0" || got.Notes != "notes for ui/v1.0.0" || len(got.Assets) != 1 || !strings.HasSuffix(got.URL, "ui/v1.0.0") {
		t.Errorf("latest = %+v", got)
	}
}

func TestLatestSkipsDraftsAndPrereleases(t *testing.T) {
	draft, pre := rel("ui/v2.0.0"), rel("ui/v1.5.0")
	draft["draft"], pre["prerelease"] = true, true
	c := serve(t, draft, pre, rel("ui/v1.0.0"))
	got, err := c.Latest(context.Background())
	if err != nil || got.Version != "1.0.0" {
		t.Errorf("latest = %+v, %v, want 1.0.0", got, err)
	}
}

func TestLatestWithNoUIReleaseSaysSo(t *testing.T) {
	if _, err := serve(t, rel("engine/v0.17.0")).Latest(context.Background()); err != ErrNone {
		t.Errorf("err = %v, want ErrNone", err)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.0.0", "0.9.2", true},
		{"0.10.0", "0.9.2", true},
		{"0.9.2", "0.9.2", false},
		{"0.9.1", "0.9.2", false},
		{"1.0.0", "dev", true},
		{"1.0.0-rc.1", "1.0.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPickChoosesTheUpdaterArtifactNotTheInstaller(t *testing.T) {
	assets := []Asset{
		{Name: "Mimic-1.0.0-macos-universal.dmg"},
		{Name: "Mimic-1.0.0-macos-universal.zip"},
		{Name: "Mimic-1.0.0-windows-amd64-installer.exe"},
		{Name: "Mimic-1.0.0-windows-amd64.zip"},
		{Name: "Mimic-1.0.0-windows-arm64.zip"},
		{Name: "mimic_1.0.0_amd64.deb"},
		{Name: "Mimic-1.0.0-linux-amd64.AppImage"},
		{Name: "Mimic-1.0.0-linux-arm64.AppImage"},
		{Name: "SHA256SUMS"},
	}
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "arm64", "Mimic-1.0.0-macos-universal.zip"},
		{"darwin", "amd64", "Mimic-1.0.0-macos-universal.zip"},
		{"windows", "amd64", "Mimic-1.0.0-windows-amd64.zip"},
		{"windows", "arm64", "Mimic-1.0.0-windows-arm64.zip"},
		{"linux", "amd64", "Mimic-1.0.0-linux-amd64.AppImage"},
		{"linux", "arm64", "Mimic-1.0.0-linux-arm64.AppImage"},
		{"linux", "riscv64", ""},
		{"freebsd", "amd64", ""},
	} {
		got := Pick(assets, c.goos, c.goarch)
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%s/%s picked %s, want nothing", c.goos, c.goarch, got.Name)
		case c.want != "" && (got == nil || got.Name != c.want):
			t.Errorf("%s/%s picked %v, want %s", c.goos, c.goarch, got, c.want)
		}
	}
	if Pick(nil, "darwin", "arm64") != nil {
		t.Error("a release with no files offered an update")
	}
}

func TestDigestAndSignature(t *testing.T) {
	c := serve(t, rel("ui/v1.0.0", "Mimic-1.0.0-macos-universal.zip", "Mimic-1.0.0-macos-universal.zip.sig", "Mimic-1.0.0-linux-amd64.AppImage", "SHA256SUMS"))
	r, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d, err := c.Digest(context.Background(), r, "Mimic-1.0.0-macos-universal.zip"); err != nil || len(d) != 32 || d[0] != 0xaa {
		t.Errorf("digest = %x, %v", d, err)
	}
	if d, err := c.Digest(context.Background(), r, "Mimic-1.0.0-linux-amd64.AppImage"); err != nil || len(d) != 32 {
		t.Errorf("a starred name: %x, %v", d, err)
	}
	if d, err := c.Digest(context.Background(), r, "Mimic-1.0.0-windows-amd64.zip"); err != nil || d != nil {
		t.Errorf("an unlisted file: %x, %v, want nothing", d, err)
	}
	if s, err := c.Signature(context.Background(), r, "Mimic-1.0.0-macos-universal.zip"); err != nil || len(s) != 64 {
		t.Errorf("signature = %d bytes, %v", len(s), err)
	}
	if s, err := c.Signature(context.Background(), r, "Mimic-1.0.0-linux-amd64.AppImage"); err != nil || s != nil {
		t.Errorf("an unsigned file: %v, %v, want nothing", s, err)
	}
}

func TestDownloadReportsProgress(t *testing.T) {
	c := serve(t, rel("ui/v1.0.0", "Mimic-1.0.0-macos-universal.zip"))
	r, _ := c.Latest(context.Background())
	var sb strings.Builder
	var last, total int64
	err := c.Download(context.Background(), &r.Assets[0], &sb, func(w, t int64) { last, total = w, t })
	if err != nil || sb.String() != "0123456789" || last != 10 || total != 10 {
		t.Errorf("download = %q, %d of %d, %v", sb.String(), last, total, err)
	}
}
