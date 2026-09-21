package card

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveFixture returns a client pointed at a test server that answers the
// named-card endpoint with the given fixture file
func serveFixture(t *testing.T, fixture string) *Client {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cards/named" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithBaseURL(srv.URL))
}

func TestFetchByName_SingleFace(t *testing.T) {
	c := serveFixture(t, "lightning_bolt.json")
	d, err := c.FetchByName(context.Background(), "lightning bolt")
	if err != nil {
		t.Fatalf("FetchByName: %v", err)
	}
	if d.Name != "Lightning Bolt" {
		t.Errorf("Name = %q, want Lightning Bolt", d.Name)
	}
	if d.ManaCost != "{R}" {
		t.Errorf("ManaCost = %q, want {R}", d.ManaCost)
	}
	if d.TypeLine != "Instant" {
		t.Errorf("TypeLine = %q, want Instant", d.TypeLine)
	}
	if len(d.ColorIdentity) != 1 || d.ColorIdentity[0] != Red {
		t.Errorf("ColorIdentity = %v, want [R]", d.ColorIdentity)
	}
	if d.SetCode != "2x2" {
		t.Errorf("SetCode = %q, want 2x2", d.SetCode)
	}
	if d.ArtworkURL == "" {
		t.Error("ArtworkURL is empty")
	}
}

func TestFetchByName_DoubleFacedFallsBackToFront(t *testing.T) {
	c := serveFixture(t, "delver.json")
	d, err := c.FetchByName(context.Background(), "delver")
	if err != nil {
		t.Fatalf("FetchByName: %v", err)
	}
	if d.Name != "Delver of Secrets" {
		t.Errorf("Name = %q, want front-face Delver of Secrets", d.Name)
	}
	if d.TypeLine != "Creature — Human Wizard" {
		t.Errorf("TypeLine = %q, want front face", d.TypeLine)
	}
	if d.ManaCost != "{U}" {
		t.Errorf("ManaCost = %q, want {U}", d.ManaCost)
	}
	if d.Power != "1" || d.Toughness != "1" {
		t.Errorf("P/T = %q/%q, want 1/1", d.Power, d.Toughness)
	}
	// ColorIdentity stays shared at the top level, not read from the face
	if len(d.ColorIdentity) != 1 || d.ColorIdentity[0] != Blue {
		t.Errorf("ColorIdentity = %v, want [U]", d.ColorIdentity)
	}
	if !strings.Contains(d.ArtworkURL, "delver") {
		t.Errorf("ArtworkURL = %q, want front-face art", d.ArtworkURL)
	}
}

func TestFetchByName_ProducedMana(t *testing.T) {
	// ProducedMana is captured in order and keeps Colorless, which a land's
	// frame logic drops but the data model preserves.
	c := serveFixture(t, "produced_land.json")
	d, err := c.FetchByName(context.Background(), "test ramp land")
	if err != nil {
		t.Fatalf("FetchByName: %v", err)
	}
	want := []Color{Blue, Green, Colorless}
	if len(d.ProducedMana) != len(want) {
		t.Fatalf("ProducedMana = %v, want %v", d.ProducedMana, want)
	}
	for i, c := range want {
		if d.ProducedMana[i] != c {
			t.Errorf("ProducedMana[%d] = %q, want %q", i, d.ProducedMana[i], c)
		}
	}
}

func TestFetchByName_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(WithBaseURL(srv.URL))
	if _, err := c.FetchByName(context.Background(), "nonesuch"); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestFetchByName_BoundedBody(t *testing.T) {
	// A response larger than the limit must error rather than silently decode
	// truncated bytes
	big := `{"name":"` + strings.Repeat("x", 4096) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(big))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(WithBaseURL(srv.URL), WithMaxCardBytes(1024))
	_, err := c.FetchByName(context.Background(), "huge")
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error = %v, want a byte-limit error", err)
	}
}

func TestFetchArt(t *testing.T) {
	// A 2x2 red PNG served as the art crop
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	got, err := c.FetchArt(context.Background(), &Data{Name: "T", ArtworkURL: srv.URL})
	if err != nil {
		t.Fatalf("FetchArt: %v", err)
	}
	if b := got.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Errorf("art bounds = %v, want 2x2", b)
	}
}

func TestFetchArt_NoURL(t *testing.T) {
	c := NewClient()
	if _, err := c.FetchArt(context.Background(), &Data{Name: "T"}); err == nil {
		t.Fatal("expected error when ArtworkURL is empty, got nil")
	}
}

func TestFetchArt_BoundedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), 4096))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(WithMaxArtBytes(1024))
	_, err := c.FetchArt(context.Background(), &Data{Name: "T", ArtworkURL: srv.URL})
	if err == nil {
		t.Fatal("expected error for oversized art, got nil")
	}
}
