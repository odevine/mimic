package run

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/mpcfill"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/rules"
	"github.com/odevine/mimic/ui/internal/workspace"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func newService(ws *workspace.Workspace) *Service {
	return New(ws, func(string) error { return nil })
}

// waitRun blocks until the latest run finishes
func waitRun(t *testing.T, ws *workspace.Workspace) batch.View {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for ws.Run.Active() {
		if time.Now().After(deadline) {
			t.Fatal("run did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The report is written just after finished is set
	run, _ := ws.Run.Current()
	for range 100 {
		if v := run.View(); v.Report != "" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	return run.View()
}

func customRunRow(name, set, cn string) batch.Row {
	return batch.Row{Qty: 1, Base: card.Data{Name: name, TypeLine: "Creature", SetCode: set, CollectorNumber: cn}}
}

func transformRow(front, back card.Face) batch.Row {
	return batch.Row{Qty: 1, Base: card.Data{
		Name: front.Name, TypeLine: front.TypeLine, Power: front.Power, Toughness: front.Toughness,
		SetCode: "isd", CollectorNumber: "51", Layout: "transform", Faces: []card.Face{front, back},
	}}
}

func TestRunWritesFilesAndReport(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	dir := filepath.Join(t.TempDir(), "out")
	rows := []batch.Row{
		customRunRow("Sol Ring", "c21", "263"),
		customRunRow("Sol Ring", "c21", "263"),
		{Qty: 4, Base: card.Data{Name: "Fire // Ice", TypeLine: "Instant"}, Fields: map[string]string{"power": "1"}},
	}
	v, err := svc.Start(Request{Rows: rows, OutDir: dir, Label: "test deck"})
	if err != nil {
		t.Fatal(err)
	}
	if v.ID == "" || len(v.Cards) != 3 {
		t.Fatalf("view = %+v", v)
	}
	v = waitRun(t, ws)

	want := []string{"Sol Ring [C21-263].jpg", "Sol Ring [C21-263] (2).jpg", "Fire - Ice.jpg"}
	for i, c := range v.Cards {
		if c.Status != batch.StatusDone || c.File != want[i] {
			t.Errorf("card %d = %+v, want done as %q", i, c, want[i])
			continue
		}
		f, err := os.Open(filepath.Join(dir, c.File))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := jpeg.DecodeConfig(f); err != nil {
			t.Errorf("%s is not a JPEG: %v", c.File, err)
		}
		f.Close()
	}

	raw, err := os.ReadFile(filepath.Join(dir, v.Report))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var rep batch.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if v.Warmup == 0 || v.WarmupCards == 0 || rep.Warmup != v.Warmup || rep.WarmupCards != v.WarmupCards {
		t.Errorf("warm-up: view %d ms over %d cards, report %d ms over %d cards", v.Warmup, v.WarmupCards, rep.Warmup, rep.WarmupCards)
	}
	if rep.Counts[batch.StatusDone] != 3 || rep.Cards[2].Qty != 4 || rep.Cards[2].Fields["power"] != "1" || rep.Label != "test deck" {
		t.Errorf("report = %+v", rep)
	}

	// A finished card is served from the file the run wrote
	if path, err := svc.File(v.ID, 0); err != nil || filepath.Ext(path) != ".jpg" {
		t.Errorf("file 0 = %q, %v", path, err)
	}

	// A rerun into the same folder overwrites rather than adding copies
	if _, err := svc.Start(Request{Rows: rows[:1], OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, ws)
	matches, _ := filepath.Glob(filepath.Join(dir, "Sol Ring*.jpg"))
	if len(matches) != 2 {
		t.Errorf("after a rerun found %v", matches)
	}
}

func TestRunWritesPNGWhenChosen(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	ws.Prefs.SetSettings(prefs.Settings{ImageFormat: "png", PNGCompression: "fast"})
	dir := t.TempDir()
	if _, err := svc.Start(Request{Rows: []batch.Row{customRunRow("Sol Ring", "c21", "263")}, OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	v := waitRun(t, ws)
	if v.Format != batch.FormatPNG || len(v.Cards) != 1 || v.Cards[0].File != "Sol Ring [C21-263].png" || v.Cards[0].Status != batch.StatusDone {
		t.Fatalf("view = %+v", v)
	}
	f, err := os.Open(filepath.Join(dir, v.Cards[0].File))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := png.DecodeConfig(f); err != nil {
		t.Errorf("not a PNG: %v", err)
	}
}

func TestRunRefusesSecondRun(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	ws.Run.Set(batch.New(nil, batch.Options{ID: "run-9"}), nil)
	_, err := svc.Start(Request{Rows: []batch.Row{customRunRow("A", "", "")}, OutDir: t.TempDir()})
	if apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("second run: %v, want a conflict", err)
	}
}

func TestRunStop(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	var rows []batch.Row
	for range 40 {
		rows = append(rows, customRunRow("Card", "", ""))
	}
	v, err := svc.Start(Request{Rows: rows, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(v.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	v = waitRun(t, ws)
	counts := batch.Counts(v.Cards)
	if !v.Stopped || counts[batch.StatusSkipped] == 0 || counts[batch.StatusQueued] != 0 {
		t.Errorf("after stop: stopped=%v counts=%v", v.Stopped, counts)
	}
	if err := svc.Stop("run-nope"); apierr.KindOf(err) != apierr.NotFound {
		t.Errorf("stopping an unknown run: %v, want not found", err)
	}
}

func TestRunRejectsBadOutput(t *testing.T) {
	svc := newService(workspacetest.NewSmall(t))
	for _, dir := range []string{"", "relative/path"} {
		if _, err := svc.Start(Request{Rows: []batch.Row{customRunRow("A", "", "")}, OutDir: dir}); apierr.KindOf(err) != apierr.BadRequest {
			t.Errorf("outDir %q: %v, want a bad request", dir, err)
		}
	}
}

func TestRetryWithNoFailuresIsRefused(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	v, err := svc.Start(Request{Rows: []batch.Row{customRunRow("Good", "", "")}, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, ws)
	if _, err := svc.Retry(v.ID); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("retry with nothing failed: %v, want a bad request", err)
	}
}

func TestRunMarksUnsupportedCards(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	dir := t.TempDir()
	rows := []batch.Row{
		customRunRow("Grizzly Bears", "", ""),
		{Qty: 1, Base: card.Data{Name: "Jace", TypeLine: "Legendary Planeswalker — Jace", ArtworkURL: "http://127.0.0.1:1/never-fetched.jpg"}},
	}
	if _, err := svc.Start(Request{Rows: rows, OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	v := waitRun(t, ws)

	if v.Cards[0].Status != batch.StatusDone {
		t.Errorf("bears = %+v", v.Cards[0])
	}
	// Refused before the art fetch, which would otherwise fail on the bad URL
	if c := v.Cards[1]; c.Status != batch.StatusUnsupported || !strings.Contains(c.Err, "planeswalker") {
		t.Errorf("jace = %+v", c)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "Jace*.png")); len(matches) != 0 {
		t.Errorf("wrote %v for an unsupported card", matches)
	}
}

func TestRunRendersEveryFace(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := newService(ws)
	dir := t.TempDir()
	rows := []batch.Row{
		transformRow(card.Face{Name: "Delver of Secrets", TypeLine: "Creature — Human Wizard", Power: "1", Toughness: "1"},
			card.Face{Name: "Insectile Aberration", TypeLine: "Creature — Human Insect", Power: "3", Toughness: "2"}),
		transformRow(card.Face{Name: "Nissa, Vastwood Seer", TypeLine: "Legendary Creature — Elf Scout", Power: "2", Toughness: "2"},
			card.Face{Name: "Nissa, Sage Animist", TypeLine: "Legendary Planeswalker — Nissa", ArtworkURL: "http://127.0.0.1:1/never-fetched.jpg"}),
	}
	if _, err := svc.Start(Request{Rows: rows, OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	v := waitRun(t, ws)

	if len(v.Cards) != 4 {
		t.Fatalf("run has %d cards, want one per face: %+v", len(v.Cards), v.Cards)
	}
	want := []struct {
		name, status, file string
		face               int
	}{
		{"Delver of Secrets", batch.StatusDone, "Delver of Secrets [ISD-51].jpg", 0},
		{"Insectile Aberration", batch.StatusDone, "Insectile Aberration [ISD-51].jpg", 1},
		{"Nissa, Vastwood Seer", batch.StatusDone, "Nissa, Vastwood Seer [ISD-51].jpg", 0},
		{"Nissa, Sage Animist", batch.StatusUnsupported, "", 1},
	}
	for i, w := range want {
		c := v.Cards[i]
		if c.Name != w.name || c.Status != w.status || c.File != w.file || c.Face != w.face {
			t.Errorf("card %d = %+v, want %+v", i, c, w)
		}
		if w.file != "" {
			if _, err := os.Stat(filepath.Join(dir, w.file)); err != nil {
				t.Errorf("card %d: %v", i, err)
			}
		}
	}
}

func TestLatestIsNilBeforeAnyRun(t *testing.T) {
	if v := newService(workspacetest.NewSmall(t)).Latest(); v != nil {
		t.Errorf("latest = %+v, want nil", v)
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pngHeader is a PNG of no pixels that claims to be w by h, which is all an
// upload's size checks read
func pngHeader(w, h int) []byte {
	ihdr := binary.BigEndian.AppendUint32([]byte("IHDR"), uint32(w))
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h))
	ihdr = append(ihdr, 8, 0, 0, 0, 0)
	b := binary.BigEndian.AppendUint32([]byte("\x89PNG\r\n\x1a\n"), uint32(len(ihdr)-4))
	b = append(b, ihdr...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(ihdr))
}

func setCardback(svc *Service, name string, body []byte) (MPCInfo, error) {
	return svc.SetCardbackImage(name, bytes.NewReader(body))
}

func TestCardbackUpload(t *testing.T) {
	svc := newService(workspacetest.NewSmall(t))
	if _, err := setCardback(svc, "x", []byte("not an image")); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("non-image: %v, want a bad request", err)
	}
	if _, err := setCardback(svc, "x", pngHeader(8000, 8001)); apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "too large to scale") {
		t.Errorf("too large to scale: %v", err)
	}
	if _, err := setCardback(svc, "x", pngBytes(t, 1200, 1100)); apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "taller") {
		t.Errorf("landscape: %v", err)
	}
	if _, err := setCardback(svc, "x", bytes.Repeat([]byte{0}, MaxCardbackUpload+1)); apierr.KindOf(err) != apierr.TooLarge {
		t.Errorf("over the upload limit: %v, want too large", err)
	}

	info, err := setCardback(svc, "House Back.jpg", pngBytes(t, 8, 1110))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if info.Cardback == nil || info.Cardback.Name != "House Back" || info.Cardback.DPI != 300 || info.MaxCards != mpcfill.MaxProjectSize {
		t.Errorf("info = %+v", info)
	}

	// Over 1500 DPI is scaled down to it, since the website hides anything more
	info, err = setCardback(svc, "Tall", pngBytes(t, 40, 5569))
	if err != nil || info.Cardback == nil || info.Cardback.Height != maxCardbackHeight || info.Cardback.DPI != mpcfill.MaxDPI {
		t.Errorf("over 1500 DPI: %v %+v", err, info.Cardback)
	}

	// A refused upload keeps the stored cardback
	setCardback(svc, "x", []byte("junk"))
	path, err := svc.CardbackFile()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if cfg, err := png.DecodeConfig(f); err != nil || cfg.Height != maxCardbackHeight {
		t.Errorf("stored cardback: %v %+v", err, cfg)
	}
}

func TestSetCardbackFromFile(t *testing.T) {
	svc := newService(workspacetest.NewSmall(t))
	path := filepath.Join(t.TempDir(), "Mine.png")
	if err := os.WriteFile(path, pngBytes(t, 8, 11), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := svc.SetCardback(path)
	if err != nil || info.Cardback == nil || info.Cardback.Name != "Mine" {
		t.Errorf("SetCardback = %+v, %v, want the card named for its file", info.Cardback, err)
	}
}

func TestMPCRunWritesProject(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	setCardback(svc, "Back.png", pngBytes(t, 8, 11))
	dir := filepath.Join(t.TempDir(), "goblin tokens")
	rows := []batch.Row{
		{Qty: 3, Base: card.Data{Name: "Island", TypeLine: "Land"}},
		{Qty: 1, Base: card.Data{Name: "Fire // Ice", TypeLine: "Instant"}},
		{Qty: 2, Base: card.Data{Name: "Island", TypeLine: "Land"}},
	}
	v, err := svc.Start(Request{Rows: rows, OutDir: dir, MPC: &MPCOptions{Stock: string(mpcfill.S33), Foil: true}})
	if err != nil || !v.MPC {
		t.Fatalf("start: %v %+v", err, v)
	}
	v = waitRun(t, ws)

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

	if _, err := svc.File(v.ID, 1); err != nil {
		t.Errorf("image from a subfolder: %v", err)
	}

	// Three fronts, the cardback and cards.xml are there to warn about
	if got, err := svc.MPCFolder(dir); err != nil || got["existing"] != 5 {
		t.Errorf("folder: %v %v", got, err)
	}
}

func TestMPCRunPrintsBothFaces(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := newService(ws)
	dir := t.TempDir()
	rows := []batch.Row{transformRow(
		card.Face{Name: "Delver of Secrets", TypeLine: "Creature — Human Wizard", Power: "1", Toughness: "1"},
		card.Face{Name: "Insectile Aberration", TypeLine: "Creature — Human Insect", Power: "3", Toughness: "2"},
	)}
	rows[0].Qty = 2
	// Every card is double-faced, so the project needs no cardback
	if _, err := svc.Start(Request{Rows: rows, OutDir: dir, MPC: &MPCOptions{}}); err != nil {
		t.Fatal(err)
	}
	v := waitRun(t, ws)
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
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	row := customRunRow("A", "", "")
	start := func(rows []batch.Row, opts MPCOptions) error {
		_, err := svc.Start(Request{Rows: rows, OutDir: t.TempDir(), MPC: &opts})
		return err
	}
	if err := start([]batch.Row{row}, MPCOptions{}); apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "cardback") {
		t.Errorf("no cardback: %v", err)
	}
	setCardback(svc, "Back", pngBytes(t, 8, 11))
	big := []batch.Row{{Qty: 600, Base: row.Base}, {Qty: 13, Base: row.Base}}
	if err := start(big, MPCOptions{}); apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "has 613") {
		t.Errorf("613 cards: %v", err)
	}
	if err := start([]batch.Row{row}, MPCOptions{Stock: string(mpcfill.P10), Foil: true}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("foil plastic: %v", err)
	}
	if err := start([]batch.Row{row}, MPCOptions{Stock: "S30"}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("unknown stock: %v", err)
	}
	// Only normal is installed, so neither face of a transform card renders
	dfc := transformRow(card.Face{Name: "Delver of Secrets", TypeLine: "Creature"}, card.Face{Name: "Insectile Aberration", TypeLine: "Creature"})
	if err := start([]batch.Row{row, dfc}, MPCOptions{}); apierr.KindOf(err) != apierr.BadRequest || !strings.Contains(err.Error(), "Delver of Secrets cannot go") {
		t.Errorf("unsupported face: %v", err)
	}
}

func TestOnFinishedHearsHowARunEnded(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	got := make(chan batch.View, 1)
	svc.OnFinished(func(v batch.View) { got <- v })
	if _, err := svc.Start(Request{Rows: []batch.Row{customRunRow("Sol Ring", "c21", "263")}, OutDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if v.ID == "" || len(v.Cards) != 1 || v.Cards[0].Status != batch.StatusDone {
			t.Errorf("finished view = %+v", v)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("OnFinished was not called")
	}
	waitRun(t, ws)
}

func TestGlobalRulesFillInWhatTheRowDidNotSet(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := newService(ws)
	ws.Prefs.SetRules([]rules.Rule{{
		ID:   "pt",
		When: []rules.Condition{{Field: "typeLine", Op: rules.OpContains, Value: "creature"}},
		Then: []rules.Action{{Op: rules.ActionSet, Field: "power", Value: "4"}, {Op: rules.ActionSet, Field: "toughness", Value: "4"}},
	}})
	rows := []batch.Row{
		customRunRow("Bear", "", ""),
		{Qty: 1, Base: card.Data{Name: "Own Power", TypeLine: "Creature"}, Fields: map[string]string{"power": "7"}},
		{Qty: 1, Base: card.Data{Name: "Spell", TypeLine: "Instant"}},
	}
	dir := t.TempDir()
	if _, err := svc.Start(Request{Rows: rows, OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	v := waitRun(t, ws)
	raw, err := os.ReadFile(filepath.Join(dir, v.Report))
	if err != nil {
		t.Fatal(err)
	}
	var rep batch.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	got := func(i int) map[string]string { return rep.Cards[i].Fields }
	if got(0)["power"] != "4" || got(0)["toughness"] != "4" {
		t.Errorf("the rule did not apply to the creature: %v", got(0))
	}
	if got(1)["power"] != "7" || got(1)["toughness"] != "4" {
		t.Errorf("the row's own power was overwritten, or toughness was not filled in: %v", got(1))
	}
	if len(got(2)) != 0 {
		t.Errorf("a rule changed a card it does not match: %v", got(2))
	}
}
