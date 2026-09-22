package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/template"
)

// resolutionServer builds a server over placeholder assets with prefs in a
// temp dir, which is enough for everything the resolution endpoints touch
func resolutionServer(t *testing.T) *server {
	t.Helper()
	withEmptyAssetChain(t)

	at, _, err := resolveActiveTemplate("normal")
	if err != nil {
		t.Fatalf("resolveActiveTemplate: %v", err)
	}
	t.Cleanup(at.cleanup)

	pipe := &renderPipeline{}
	pipe.install(at)
	s := &server{pipe: pipe, prefs: loadPrefs()}
	s.routes()
	return s
}

// nativeDPI is what the active template reports as its authored resolution
func nativeDPI(t *testing.T, s *server) int {
	t.Helper()
	m, err := s.pipe.manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return m.NativeDPI()
}

func TestResolutionsDefault(t *testing.T) {
	s := resolutionServer(t)
	got, err := s.resolutions()
	if err != nil {
		t.Fatalf("resolutions: %v", err)
	}
	native := nativeDPI(t, s)

	// Nothing has been chosen, so the preview takes the default and the output
	// takes the template's own resolution
	if want := min(defaultPreviewDPI, native); got.Preview.DPI != want {
		t.Errorf("preview dpi = %d, want %d", got.Preview.DPI, want)
	}
	if got.Output.DPI != native || !got.Output.Native {
		t.Errorf("output = %+v, want the native %d dpi", got.Output, native)
	}
	if got.MaxDPI != native {
		t.Errorf("maxDpi = %d, want %d", got.MaxDPI, native)
	}
	if len(got.Presets) == 0 {
		t.Error("no presets offered")
	}
	if last := got.Presets[len(got.Presets)-1]; !last.Native {
		t.Errorf("last preset = %+v, want the native one", last)
	}
}

func TestRenderDPIByTarget(t *testing.T) {
	s := resolutionServer(t)
	native := nativeDPI(t, s)
	s.prefs.setResolution(96, 0)

	if got := s.renderDPI(targetPreview); got != 96 {
		t.Errorf("preview dpi = %d, want 96", got)
	}
	// Zero output means the template's own resolution, which the engine
	// resolves from the manifest rather than the preference
	if got := s.renderDPI(targetOutput); got != 0 {
		t.Errorf("output dpi = %d, want 0 for the template's own", got)
	}
	m, err := s.pipe.manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if got := m.ClampDPI(s.renderDPI(targetOutput)); got != native {
		t.Errorf("clamped output dpi = %d, want the native %d", got, native)
	}
}

func TestSetResolutionClampsAndPersists(t *testing.T) {
	s := resolutionServer(t)
	native := nativeDPI(t, s)

	body := `{"previewDpi":1,"outputDpi":999999}`
	req := httptest.NewRequest(http.MethodPost, "/api/resolution", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var got resolutionSettings
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	// A dpi under the floor clamps up, and one past the template's own clamps
	// back to it
	if want := min(template.MinDPI, native); got.Preview.DPI != want {
		t.Errorf("preview dpi = %d, want %d", got.Preview.DPI, want)
	}
	if got.Output.DPI != native {
		t.Errorf("output dpi = %d, want the native %d", got.Output.DPI, native)
	}

	// An output at or past native is stored as zero, so it follows a later
	// template rather than pinning this one's number
	if _, output := s.prefs.resolution(); output != 0 {
		t.Errorf("stored output dpi = %d, want 0", output)
	}
}

func TestResolutionRoundTrip(t *testing.T) {
	s := resolutionServer(t)
	native := nativeDPI(t, s)
	if native <= template.MinDPI*2 {
		t.Skipf("template native dpi %d leaves no room for an in-range choice", native)
	}
	pick := native / 2

	body := `{"previewDpi":` + strconv.Itoa(pick) + `,"outputDpi":` + strconv.Itoa(pick) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/resolution", strings.NewReader(body))
	s.mux.ServeHTTP(httptest.NewRecorder(), req)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/resolution", nil))
	var got resolutionSettings
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Preview.DPI != pick || got.Output.DPI != pick {
		t.Errorf("round trip = %d/%d, want %d/%d", got.Preview.DPI, got.Output.DPI, pick, pick)
	}
	if got.Preview.Width <= 0 || got.Preview.Height <= 0 {
		t.Errorf("preview size = %dx%d, want a positive size", got.Preview.Width, got.Preview.Height)
	}
	if got.Output.Native {
		t.Error("an output below native was reported as native")
	}
}
