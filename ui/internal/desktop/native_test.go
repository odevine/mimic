package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/releases"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestRunSummaryNamesNoCards(t *testing.T) {
	cards := []batch.Card{{Name: "Sol Ring", Status: batch.StatusDone}, {Name: "Black Lotus", Status: batch.StatusDone}, {Name: "Mox", Status: batch.StatusFailed}}
	title, body := runSummary(batch.View{ID: "run-1", Cards: cards})
	if title != "Run finished" || body != "2 cards rendered, 1 failed" {
		t.Errorf("summary = %q, %q", title, body)
	}
	if strings.Contains(body, "Sol Ring") || strings.Contains(body, "Mox") {
		t.Errorf("the text names a card: %q", body)
	}
	title, body = runSummary(batch.View{Stopped: true, Cards: []batch.Card{{Status: batch.StatusDone}, {Status: batch.StatusSkipped}, {Status: batch.StatusSkipped}}})
	if title != "Run stopped" || body != "1 card rendered, 2 skipped" {
		t.Errorf("stopped summary = %q, %q", title, body)
	}
}

func TestCheckDueOncePerDay(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name    string
		last    time.Time
		enabled bool
		want    bool
	}{
		{"never checked", time.Time{}, true, true},
		{"an hour ago", now.Add(-time.Hour), true, false},
		{"just over a day ago", now.Add(-25 * time.Hour), true, true},
		{"turned off", time.Time{}, false, false},
	} {
		if got := checkDue(now, c.last, c.enabled); got != c.want {
			t.Errorf("%s: checkDue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestProgressOfReadsEveryShapeTheEventArrivesIn(t *testing.T) {
	want := updater.Progress{Written: 5, Total: 10}
	for name, in := range map[string]any{
		"value":   want,
		"pointer": &want,
		"list":    []any{want},
		"decoded": map[string]any{"written": 5, "total": 10},
		"nested":  []any{&want},
	} {
		if got, ok := progressOf(in); !ok || got.Written != 5 || got.Total != 10 {
			t.Errorf("%s: %+v, %v", name, got, ok)
		}
	}
	if _, ok := progressOf("nope"); ok {
		t.Error("a string read as progress")
	}
}

// releaseServer answers the GitHub release list with the given releases
func releaseServer(t *testing.T, list ...map[string]any) *provider {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/odevine/mimic/releases") {
			raw, _ := json.Marshal(list)
			w.Write([]byte(strings.ReplaceAll(string(raw), "SERVER", srv.URL)))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			w.Write([]byte(strings.Repeat("ab", 32) + "  Mimic-1.0.0-macos-universal.zip\n"))
		default:
			w.Write([]byte("bytes"))
		}
	}))
	t.Cleanup(srv.Close)
	return &provider{client: &releases.Client{HTTP: srv.Client(), Repo: "odevine/mimic", BaseURL: srv.URL}}
}

func uiRelease(tag string, assets ...string) map[string]any {
	var as []map[string]any
	for _, a := range assets {
		as = append(as, map[string]any{"name": a, "size": 5, "browser_download_url": "SERVER/dl/" + a})
	}
	return map[string]any{"tag_name": tag, "name": tag, "body": "notes", "html_url": "https://example.test/" + tag, "assets": as}
}

func TestProviderOffersTheNewerUIReleaseWithItsDigest(t *testing.T) {
	p := releaseServer(t,
		uiRelease("engine/v9.9.9"),
		uiRelease("ui/v1.0.0", "Mimic-1.0.0-macos-universal.dmg", "Mimic-1.0.0-macos-universal.zip", "SHA256SUMS"),
	)
	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.9.2", Platform: "darwin", Arch: "arm64"})
	if err != nil || rel == nil {
		t.Fatalf("check = %+v, %v", rel, err)
	}
	if rel.Version != "1.0.0" || rel.Artifact.Filename != "Mimic-1.0.0-macos-universal.zip" || rel.Artifact.Filetype != "zip" || rel.Notes != "notes" {
		t.Errorf("release = %+v", rel)
	}
	if rel.Verification == nil || rel.Verification.DigestAlgo != "sha256" || len(rel.Verification.Digest) != 32 || rel.Verification.Signature != nil {
		t.Errorf("verification = %+v, want a digest and no signature", rel.Verification)
	}
	if rel.Metadata["pageURL"] != "https://example.test/ui/v1.0.0" {
		t.Errorf("metadata = %v", rel.Metadata)
	}

	var got strings.Builder
	var seen int64
	if err := p.Download(context.Background(), rel, &got, func(w, _ int64) { seen = w }); err != nil || got.String() != "bytes" || seen != 5 {
		t.Errorf("download = %q, %d, %v", got.String(), seen, err)
	}
}

func TestProviderSaysNothingWhenThereIsNothingToInstall(t *testing.T) {
	check := func(p *provider, current, goos, arch string) *updater.Release {
		t.Helper()
		rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: current, Platform: goos, Arch: arch})
		if err != nil {
			t.Fatal(err)
		}
		return rel
	}
	p := releaseServer(t, uiRelease("ui/v1.0.0", "Mimic-1.0.0-macos-universal.zip"))
	if check(p, "1.0.0", "darwin", "arm64") != nil {
		t.Error("the same version was offered")
	}
	if check(p, "0.9.2", "windows", "amd64") != nil {
		t.Error("a release with no file for this platform was offered")
	}
	// Published before its files were uploaded
	if check(releaseServer(t, uiRelease("ui/v1.0.0")), "0.9.2", "darwin", "arm64") != nil {
		t.Error("a release with no files was offered")
	}
	if check(releaseServer(t, uiRelease("engine/v1.0.0")), "0.9.2", "darwin", "arm64") != nil {
		t.Error("an engine release counted as an update")
	}
}
