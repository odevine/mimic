package testbuild

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/fakeupstream"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/scryfall"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/testassets"
)

// restoreGlobals puts back what Apply changes, since it works on process state
func restoreGlobals(t *testing.T) {
	t.Helper()
	dir, bases := workspace.UserConfigDir, pipeline.LooseDirBases
	tr := http.DefaultTransport.(*http.Transport)
	dial, dialTLS, proxy, h2 := tr.DialContext, tr.DialTLSContext, tr.Proxy, tr.ForceAttemptHTTP2
	t.Cleanup(func() {
		workspace.UserConfigDir, pipeline.LooseDirBases = dir, bases
		tr.DialContext, tr.DialTLSContext, tr.Proxy, tr.ForceAttemptHTTP2 = dial, dialTLS, proxy, h2
		tr.CloseIdleConnections()
		settings.ForceLive()
	})
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestApplyNeedsAScratchFolder(t *testing.T) {
	restoreGlobals(t)
	err := Apply(envOf(nil))
	if err == nil || !strings.Contains(err.Error(), EnvHome) {
		t.Fatalf("Apply without %s = %v, want an error naming it", EnvHome, err)
	}
}

func TestApplyMovesTheConfigFolderAndIgnoresLooseAssets(t *testing.T) {
	restoreGlobals(t)
	home := t.TempDir()
	if err := Apply(envOf(map[string]string{EnvHome: home})); err != nil {
		t.Fatal(err)
	}
	got, err := workspace.UserConfigDir()
	if err != nil || got != home {
		t.Errorf("config folder = %q, %v, want %q", got, err, home)
	}
	if len(pipeline.LooseDirBases) != 0 {
		t.Errorf("loose asset roots = %v, want none", pipeline.LooseDirBases)
	}
}

func TestApplyRejectsAFeatureTheTableDoesNotList(t *testing.T) {
	restoreGlobals(t)
	err := Apply(envOf(map[string]string{EnvHome: t.TempDir(), EnvCapabilities: "flow.single,flow.nope"}))
	if err == nil || !strings.Contains(err.Error(), "flow.nope") {
		t.Fatalf("Apply with a typo = %v, want an error naming the key", err)
	}
}

func TestApplyForcesTheListedFeaturesLive(t *testing.T) {
	restoreGlobals(t)
	before := settings.New(nil).Capabilities()["single.compare"]
	if before.State == "live" {
		t.Skip("single.compare is live, so pick a gate that is not")
	}
	if err := Apply(envOf(map[string]string{EnvHome: t.TempDir(), EnvCapabilities: " single.compare , palette,"})); err != nil {
		t.Fatal(err)
	}
	caps := settings.New(nil).Capabilities()
	for _, k := range []string{"single.compare", "palette"} {
		if caps[k].State != "live" {
			t.Errorf("%s = %+v, want live", k, caps[k])
		}
	}
	if caps["flow.art"].State == "live" {
		t.Error("a feature that was not listed was forced live")
	}
}

// get fetches a URL with a client cloned from the default transport, the way
// the paced Scryfall client is built
func get(t *testing.T, url string) (int, string, error) {
	t.Helper()
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	resp, err := client.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func TestRedirectSendsEveryHostToTheFakeWithItsHostHeader(t *testing.T) {
	restoreGlobals(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host+r.URL.Path)
	}))
	defer fake.Close()
	Redirect(strings.TrimPrefix(fake.URL, "http://"))

	for url, want := range map[string]string{
		"https://api.scryfall.com/cards/named":                                      "api.scryfall.com/cards/named",
		"http://cards.scryfall.io/art_crop/x.jpg":                                   "cards.scryfall.io/art_crop/x.jpg",
		"https://api.github.com/repos/odevine/mimic/releases":                       "api.github.com/repos/odevine/mimic/releases",
		"https://raw.githubusercontent.com/odevine/mimic-templates/main/index.json": "raw.githubusercontent.com/odevine/mimic-templates/main/index.json",
	} {
		code, body, err := get(t, url)
		if err != nil || code != 200 || body != want {
			t.Errorf("GET %s = %d %q, %v, want 200 %q", url, code, body, err, want)
		}
	}
}

func TestRedirectIgnoresTheProxyEnvironment(t *testing.T) {
	restoreGlobals(t)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer fake.Close()
	Redirect(strings.TrimPrefix(fake.URL, "http://"))
	if _, body, err := get(t, "https://api.scryfall.com/"); err != nil || body != "ok" {
		t.Errorf("a proxy in the environment got in the way: %q, %v", body, err)
	}
}

func TestWithNoUpstreamEveryRequestIsRefused(t *testing.T) {
	restoreGlobals(t)
	if err := Apply(envOf(map[string]string{EnvHome: t.TempDir()})); err != nil {
		t.Fatal(err)
	}
	if code, _, err := get(t, "https://api.scryfall.com/cards/named?exact=Sol+Ring"); err == nil {
		t.Errorf("a request left the process: status %d", code)
	}
}

func TestRestoreReturnsTheTransport(t *testing.T) {
	restoreGlobals(t)
	tr := http.DefaultTransport.(*http.Transport)
	had := tr.DialTLSContext != nil
	restore := Redirect(refused)
	if tr.DialTLSContext == nil {
		t.Fatal("Redirect did not set a dialer")
	}
	restore()
	if (tr.DialTLSContext != nil) != had {
		t.Error("restore did not put the dialer back")
	}
}

// A test build must stay out of every path that makes a release. The production
// tag together with a test tag also fails to compile, which covers a recipe the
// patterns below do not anticipate
func TestReleaseRecipesBuildWithoutTheTestTags(t *testing.T) {
	files := []string{
		"../../Taskfile.yml",
		"../../build/darwin/Taskfile.yml",
		"../../build/linux/Taskfile.yml",
		"../../build/windows/Taskfile.yml",
		"../../../.github/workflows/package-ui.yml",
		"../../../.github/workflows/release-please.yml",
	}
	tags := regexp.MustCompile(`-tags[ =]+["']?([^\s"']+)`)
	for _, f := range files {
		raw, err := os.ReadFile(filepath.FromSlash(f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		for _, m := range tags.FindAllStringSubmatch(string(raw), -1) {
			for _, tag := range strings.Split(m[1], ",") {
				if tag == "server" || tag == "mcp" {
					t.Errorf("%s builds with the %s tag, which is for test builds only", f, tag)
				}
			}
		}
	}
}

// The paced client the app searches with, redirected to the fake upstream, reads
// the fixture cards and their art, which is what every end to end test relies on
func TestThePacedClientReadsCardsAndArtFromTheFakeUpstream(t *testing.T) {
	restoreGlobals(t)
	cards, err := fs.Sub(testassets.Scryfall, "scryfall")
	if err != nil {
		t.Fatal(err)
	}
	fake, err := fakeupstream.New(cards)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	Redirect(strings.TrimPrefix(srv.URL, "http://"))

	client := card.NewClient(card.WithHTTPClient(scryfall.NewHTTPClient(10 * time.Second)))
	found, err := client.Search(context.Background(), "lightning bolt")
	if err != nil || len(found) != 2 {
		t.Fatalf("Search = %d cards, %v, want the two printings", len(found), err)
	}
	if found[0].Name != "Lightning Bolt" || found[0].ManaCost != "{R}" || found[0].ArtworkURL == "" {
		t.Errorf("first card = %+v", found[0])
	}
	if img, err := client.FetchArt(context.Background(), found[0]); err != nil || img.Bounds().Dx() == 0 {
		t.Errorf("FetchArt = %v, %v", img, err)
	}
	delver, err := client.Search(context.Background(), `!"Delver of Secrets"`)
	if err != nil || len(delver) != 1 || len(delver[0].Faces) != 2 {
		t.Errorf("a double-faced card = %v, %v, want one card with two faces", delver, err)
	}
}
