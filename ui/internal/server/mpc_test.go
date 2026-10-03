package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/mpcfill"
	"github.com/odevine/mimic/ui/internal/batch"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func putCardback(s *Server, name string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/mpc/cardback?name="+name, bytes.NewReader(body)))
	return rec
}

func TestCardbackUpload(t *testing.T) {
	s := runServer(t)
	if rec := putCardback(s, "x", []byte("not an image")); rec.Code != http.StatusBadRequest {
		t.Errorf("non-image: %d, want 400", rec.Code)
	}
	if rec := putCardback(s, "x", pngBytes(t, 1, 5569)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "1510 DPI") {
		t.Errorf("over 1500 DPI: %d %s", rec.Code, rec.Body)
	}

	if rec := putCardback(s, "x", pngBytes(t, 1200, 1100)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "taller") {
		t.Errorf("landscape: %d %s", rec.Code, rec.Body)
	}
	if rec := putCardback(s, "x", bytes.Repeat([]byte{0}, maxCardbackUpload+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("over the upload limit: %d, want 413", rec.Code)
	}

	rec := putCardback(s, "House%20Back.jpg", pngBytes(t, 8, 1110))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var info mpcInfo
	json.Unmarshal(rec.Body.Bytes(), &info)
	if info.Cardback == nil || info.Cardback.Name != "House Back" || info.Cardback.DPI != 300 || info.MaxCards != mpcfill.MaxProjectSize {
		t.Errorf("info = %+v", info)
	}

	// A refused upload keeps the stored cardback
	putCardback(s, "x", []byte("junk"))
	img := httptest.NewRecorder()
	s.mux.ServeHTTP(img, httptest.NewRequest(http.MethodGet, "/api/mpc/cardback", nil))
	if cfg, err := png.DecodeConfig(img.Body); img.Code != http.StatusOK || err != nil || cfg.Height != 1110 {
		t.Errorf("stored cardback: %d %v %+v", img.Code, err, cfg)
	}
}

func TestMPCRunWritesProject(t *testing.T) {
	s := runServer(t)
	putCardback(s, "Back.png", pngBytes(t, 8, 11))
	dir := filepath.Join(t.TempDir(), "goblin tokens")
	rows := []batch.Row{
		{Qty: 3, Base: card.Data{Name: "Island", TypeLine: "Land"}},
		{Qty: 1, Base: card.Data{Name: "Fire // Ice", TypeLine: "Instant"}},
		{Qty: 2, Base: card.Data{Name: "Island", TypeLine: "Land"}},
	}
	rec, v := postRun(t, s, runBody{Rows: rows, OutDir: dir, MPC: &mpcOptions{Stock: string(mpcfill.S33), Foil: true}})
	if rec.Code != http.StatusOK || !v.MPC {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	v = waitRun(t, s)

	want := []string{"fronts/Island.png", "fronts/Fire Ice.png", "fronts/Island 2.png"}
	for i, c := range v.Cards {
		if c.Status != batch.StatusDone || filepath.ToSlash(c.File) != want[i] {
			t.Errorf("card %d = %+v, want done as %q", i, c, want[i])
		}
	}
	if v.Order != mpcfill.OrderFile {
		t.Errorf("order = %q", v.Order)
	}
	order, err := os.ReadFile(filepath.Join(dir, mpcfill.OrderFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"<quantity>6</quantity>", "<stock>(S33) Superior Smooth</stock>", "<foil>true</foil>", "<cardback>./cardback/Back.png</cardback>", "<slots>4,5</slots>"} {
		if !strings.Contains(string(order), s) {
			t.Errorf("order file lacks %s:\n%s", s, order)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, mpcfill.CardbackDir, "Back.png")); err != nil {
		t.Errorf("cardback not copied: %v", err)
	}

	img := httptest.NewRecorder()
	s.mux.ServeHTTP(img, httptest.NewRequest(http.MethodGet, "/api/run/"+v.ID+"/image/1", nil))
	if img.Code != http.StatusOK {
		t.Errorf("image from a subfolder: %d", img.Code)
	}
}

func TestMPCRunPrintsBothFaces(t *testing.T) {
	s := facesServer(t)
	dir := t.TempDir()
	rows := []batch.Row{transformRow(
		card.Face{Name: "Delver of Secrets", TypeLine: "Creature — Human Wizard", Power: "1", Toughness: "1"},
		card.Face{Name: "Insectile Aberration", TypeLine: "Creature — Human Insect", Power: "3", Toughness: "2"},
	)}
	rows[0].Qty = 2
	// Every card is double-faced, so the project needs no cardback
	rec, _ := postRun(t, s, runBody{Rows: rows, OutDir: dir, MPC: &mpcOptions{}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	v := waitRun(t, s)
	if len(v.Cards) != 2 || filepath.ToSlash(v.Cards[1].File) != "backs/Insectile Aberration.png" || v.Order != mpcfill.OrderFile {
		t.Fatalf("run = %+v", v)
	}
	order, _ := os.ReadFile(filepath.Join(dir, mpcfill.OrderFile))
	for _, s := range []string{"<id>./fronts/Delver of Secrets.png</id>", "<id>./backs/Insectile Aberration.png</id>", "<slots>0,1</slots>"} {
		if !strings.Contains(string(order), s) {
			t.Errorf("order file lacks %s:\n%s", s, order)
		}
	}
	if strings.Contains(string(order), "<cardback") {
		t.Errorf("an all double-faced project has a cardback:\n%s", order)
	}
}

func TestMPCRunRefusals(t *testing.T) {
	s := runServer(t)
	row := customRunRow("A", "", "")
	post := func(rows []batch.Row, opts mpcOptions) *httptest.ResponseRecorder {
		rec, _ := postRun(t, s, runBody{Rows: rows, OutDir: t.TempDir(), MPC: &opts})
		return rec
	}
	if rec := post([]batch.Row{row}, mpcOptions{}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "cardback") {
		t.Errorf("no cardback: %d %s", rec.Code, rec.Body)
	}
	putCardback(s, "Back", pngBytes(t, 8, 11))
	big := []batch.Row{{Qty: 600, Base: row.Base}, {Qty: 13, Base: row.Base}}
	if rec := post(big, mpcOptions{}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "has 613") {
		t.Errorf("613 cards: %d %s", rec.Code, rec.Body)
	}
	if rec := post([]batch.Row{row}, mpcOptions{Stock: string(mpcfill.P10), Foil: true}); rec.Code != http.StatusBadRequest {
		t.Errorf("foil plastic: %d", rec.Code)
	}
	if rec := post([]batch.Row{row}, mpcOptions{Stock: "S30"}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown stock: %d", rec.Code)
	}
	// Only normal is installed, so neither face of a transform card renders
	dfc := transformRow(card.Face{Name: "Delver of Secrets", TypeLine: "Creature"}, card.Face{Name: "Insectile Aberration", TypeLine: "Creature"})
	if rec := post([]batch.Row{row, dfc}, mpcOptions{}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Delver of Secrets cannot go") {
		t.Errorf("unsupported face: %d %s", rec.Code, rec.Body)
	}
}

func TestSettingsKeepOnlyKnownOutputFormats(t *testing.T) {
	s := runServer(t)
	for in, want := range map[string]string{"mpc": "mpc", "pdf": "", "": ""} {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"outputFormat":"`+in+`"}`)))
		if got := s.prefs.Settings().OutputFormat; rec.Code != http.StatusOK || got != want {
			t.Errorf("outputFormat %q stored as %q (%d), want %q", in, got, rec.Code, want)
		}
	}
}
