package studio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/draftedit"
)

// writePNG writes a w by h image of one color, with the alpha given, to a folder
// outside the draft, and returns its path
func writePNG(t *testing.T, w, h int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "in.png")
	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func readDraft(t *testing.T, dir, id, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, id, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func material(p PartView, key string) MaterialView {
	for _, m := range p.Materials {
		if m.Key == key {
			return m
		}
	}
	return MaterialView{Key: "none:" + key}
}

func part(t *testing.T, v AssetsView, name string) PartView {
	t.Helper()
	for _, p := range v.Parts {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no part %q in %+v", name, v.Parts)
	return PartView{}
}

func TestAssetsListsThePartsFontsAndRolesOfADraft(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	v, err := svc.Assets(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	bg := part(t, v, "background")
	if bg.Slot != "" || bg.Dir != "parts/background" || len(bg.Materials) != 1 {
		t.Fatalf("background = %+v", bg)
	}
	if m := bg.Materials[0]; m.Key != "any" || m.State != StateImage || m.File != "parts/background/any.png" || m.Width != 272 || m.Height != 370 || m.Bytes == 0 {
		t.Errorf("the background's image = %+v", m)
	}
	if len(v.Fonts) != 0 {
		t.Errorf("fonts = %+v", v.Fonts)
	}
	if len(v.Roles) != 6 || v.Roles[0].Role != "title" || !v.Roles[0].Standard || v.Roles[0].Default != "mimic-m15-title" || v.Roles[0].Label == "" {
		t.Errorf("roles = %+v", v.Roles)
	}
	if len(v.Builtins) != 6 {
		t.Errorf("builtins = %+v", v.Builtins)
	}
	var pin SlotView
	for _, s := range v.Slots {
		if s.Name == "pinlines" {
			pin = s
		}
	}
	if len(pin.Materials) < 20 || pin.Materials[0] != "any" {
		t.Errorf("the pinlines slot reads %v", pin.Materials)
	}
}

func TestAssetsOfADraftThatDoesNotReadIsEmptyAndNotAFailure(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	os.WriteFile(filepath.Join(dir, sum.ID, "template.json"), []byte("{"), 0o644)
	v, err := svc.Assets(sum.ID)
	if err != nil || len(v.Parts) != 0 || len(v.Roles) != 6 {
		t.Fatalf("view = %+v, %v", v, err)
	}
	if _, err := svc.Assets("nope"); err == nil {
		t.Error("assets of a draft that is not there")
	}
}

func TestImportingAnImageReplacesTheOneThereAndUndoPutsItBack(t *testing.T) {
	svc, ws, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	card := presetCard(t, "white-creature")
	was := readDraft(t, dir, sum.ID, "parts/background/any.png")
	before := renderDraft(t, svc, ws, sum.ID, card)

	in := writePNG(t, 272, 370, color.NRGBA{R: 200, G: 10, B: 10, A: 255})
	res, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "background", Material: "any", Path: in})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "background" || !res.Valid || res.Rev != 1 || len(res.Warnings) != 0 {
		t.Fatalf("result = %+v", res)
	}
	now := readDraft(t, dir, sum.ID, "parts/background/any.png")
	if bytes.Equal(now, was) {
		t.Fatal("the image was not replaced")
	}
	// The render shows the new image, which is the draft forgetting what it had decoded
	after := renderDraft(t, svc, ws, sum.ID, card)
	if samePixels(before, after) {
		t.Error("the render did not change after the image was replaced")
	}
	r, g, b, _ := after.At(5, 5).RGBA()
	if r>>8 < 150 || g>>8 > 60 || b>>8 > 60 {
		t.Errorf("a corner is %d,%d,%d, want the new red", r>>8, g>>8, b>>8)
	}

	// The inverse puts the old image back, byte for byte, and redo puts the new one
	undo, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: res.Inverse})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readDraft(t, dir, sum.ID, "parts/background/any.png"), was) {
		t.Fatal("undo did not restore the image")
	}
	if !samePixels(before, renderDraft(t, svc, ws, sum.ID, card)) {
		t.Error("the render after undo is not the render before")
	}
	if _, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: undo.Inverse}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readDraft(t, dir, sum.ID, "parts/background/any.png"), now) {
		t.Error("redo did not write the new image")
	}
}

func TestImportingIntoANewPartMakesItWithItsSlot(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	res, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "pinlines", Slot: "pinlines", Material: "w", Path: writePNG(t, 272, 370, color.NRGBA{A: 0})})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("the draft stopped loading: %+v", res.Diagnostics)
	}
	if got := readDraft(t, dir, sum.ID, "parts/pinlines/w.png"); len(got) == 0 {
		t.Fatal("the image is not in the part's folder")
	}
	var src map[string]any
	json.Unmarshal(readDraft(t, dir, sum.ID, "template.json"), &src)
	p := src["parts"].(map[string]any)["pinlines"].(map[string]any)
	if p["slot"] != "pinlines" || p["dir"] != "parts/pinlines" {
		t.Errorf("the part is %v", p)
	}

	v, _ := svc.Assets(sum.ID)
	pin := part(t, v, "pinlines")
	if m := material(pin, "w"); m.State != StateImage || m.File != "parts/pinlines/w.png" {
		t.Errorf("w = %+v", m)
	}
	if m := material(pin, "u"); m.State != StateMissing {
		t.Errorf("u = %+v", m)
	}
	if m := material(pin, "wu"); m.State != StateMissing {
		t.Errorf("wu with only w = %+v", m)
	}

	// With both colors, the pair is drawn by blending them, and an "any" image covers the rest
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "pinlines", Material: "u", Path: writePNG(t, 272, 370, color.NRGBA{A: 0})}); err != nil {
		t.Fatal(err)
	}
	v, _ = svc.Assets(sum.ID)
	if m := material(part(t, v, "pinlines"), "wu"); m.State != StateBlended || strings.Join(m.From, "") != "wu" {
		t.Errorf("wu with w and u = %+v", m)
	}
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "pinlines", Material: "any", Path: writePNG(t, 272, 370, color.NRGBA{A: 0})}); err != nil {
		t.Fatal(err)
	}
	v, _ = svc.Assets(sum.ID)
	if m := material(part(t, v, "pinlines"), "gold"); m.State != StateAny {
		t.Errorf("gold with an any image = %+v", m)
	}
}

func TestImportRefusesWhatCannotBeUsed(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	good := writePNG(t, 272, 370, color.NRGBA{A: 255})
	notPNG := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(notPNG, []byte("GIF89a not a png"), 0o644)
	for _, tt := range []struct {
		name string
		req  ImageRequest
		why  string
	}{
		{"a file that is not a PNG", ImageRequest{Part: "background", Material: "any", Path: notPNG}, "not a PNG"},
		{"a file that is not there", ImageRequest{Part: "background", Material: "any", Path: filepath.Join(t.TempDir(), "x.png")}, "Could not read"},
		{"a folder", ImageRequest{Part: "background", Material: "any", Path: t.TempDir()}, "not a file"},
		{"a bad part name", ImageRequest{Part: "../x", Material: "any", Path: good}, "not a name for a part"},
		{"a material the slot does not read", ImageRequest{Part: "background", Material: "w", Path: good}, "does not read an image for"},
		{"an unknown slot for a new part", ImageRequest{Part: "fresh", Slot: "nonsense", Material: "any", Path: good}, "not a slot"},
		{"a draft that is not there", ImageRequest{ID: "nope", Part: "background", Material: "any", Path: good}, ""},
	} {
		if tt.req.ID == "" {
			tt.req.ID = sum.ID
		}
		_, err := svc.ImportImage(tt.req)
		if err == nil || !strings.Contains(err.Error(), tt.why) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.why)
		}
	}
	// An image far too large for its pixels is refused before it is decoded in full
	big := writePNG(t, 20000, 2, color.NRGBA{A: 255})
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "background", Material: "any", Path: big}); err == nil || apierr.KindOf(err) != apierr.TooLarge {
		t.Errorf("a very wide image gave %v", err)
	}
}

func TestImportWarnsAboutWhatLooksWrongAndStillTakesTheImage(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	// Larger than the canvas, and a different size than the part's other image
	res, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "background", Material: "w", Path: writePNG(t, 300, 400, color.NRGBA{A: 255})})
	if err == nil {
		t.Fatalf("a background part does not read w, got %+v", res)
	}
	res, err = svc.ImportImage(ImageRequest{ID: sum.ID, Part: "frame", Slot: "pinlines", Material: "any", Path: writePNG(t, 300, 400, color.NRGBA{A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "larger than") || !strings.Contains(joined, "no transparent pixels") {
		t.Errorf("warnings = %q", joined)
	}
	res, err = svc.ImportImage(ImageRequest{ID: sum.ID, Part: "frame", Material: "w", Path: writePNG(t, 272, 370, color.NRGBA{A: 10})})
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(res.Warnings, "\n"); !strings.Contains(joined, "must be one size") {
		t.Errorf("warnings = %q", joined)
	}
	v, _ := svc.Assets(sum.ID)
	if p := part(t, v, "frame"); len(p.Problems) != 1 || !strings.Contains(p.Problems[0], "not one size") {
		t.Errorf("problems = %v", p.Problems)
	}
}

func TestAnImageIsRemovedWithItsVariantAndPutBackByUndo(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "glow", Slot: "pinlines", Material: "w", Path: writePNG(t, 272, 370, color.NRGBA{A: 0})}); err != nil {
		t.Fatal(err)
	}
	// A second image so the part still has one
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "glow", Material: "u", Path: writePNG(t, 272, 370, color.NRGBA{A: 0})}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: []draftedit.Op{{Op: draftedit.OpDrop, File: "parts/glow/w.png"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, sum.ID, "parts", "glow", "w.png")); !os.IsNotExist(err) {
		t.Fatalf("the image is still there: %v", err)
	}
	v, _ := svc.Assets(sum.ID)
	if m := material(part(t, v, "glow"), "w"); m.State != StateMissing {
		t.Errorf("w after removing = %+v", m)
	}
	if _, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: res.Inverse}); err != nil {
		t.Fatal(err)
	}
	if len(readDraft(t, dir, sum.ID, "parts/glow/w.png")) == 0 {
		t.Error("undo did not put the image back")
	}
}

const mana = "../../../../engine/fonts/embedded/m15/plantin-body.ttf"

func TestImportingAFontAddsItAndAssignsItsRoles(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	res, err := svc.ImportFont(FontRequest{ID: sum.ID, Path: mana, Assign: []string{"body", "body-italic"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid || res.ID == "" || !strings.HasPrefix(res.ID, "plantin") {
		t.Fatalf("result = %+v", res)
	}
	file := "font/" + res.ID + ".ttf"
	disk := readDraft(t, dir, sum.ID, file)
	want, _ := os.ReadFile(mana)
	if !bytes.Equal(disk, want) {
		t.Error("the font was not copied as it is")
	}
	var src struct {
		Fonts map[string]struct {
			File   string `json:"file"`
			Family string `json:"family"`
			SHA256 string `json:"sha256"`
		} `json:"fonts"`
		Roles map[string]struct {
			Font string `json:"font"`
		} `json:"roles"`
	}
	json.Unmarshal(readDraft(t, dir, sum.ID, "template.json"), &src)
	f := src.Fonts[res.ID]
	if f.File != file || f.Family == "" || len(f.SHA256) != 64 || src.Roles["body"].Font != res.ID || src.Roles["body-italic"].Font != res.ID {
		t.Fatalf("template.json = %+v", src)
	}

	v, _ := svc.Assets(sum.ID)
	if len(v.Fonts) != 1 || v.Fonts[0].ID != res.ID || v.Fonts[0].Missing {
		t.Errorf("fonts = %+v", v.Fonts)
	}
	for _, r := range v.Roles {
		switch r.Role {
		case "body", "body-italic":
			if r.Font != res.ID {
				t.Errorf("role %s = %+v", r.Role, r)
			}
		case "title":
			if r.Font != "" || r.Default == "" {
				t.Errorf("role title = %+v", r)
			}
		}
	}

	// A second import of the same font is a second entry, not a collision
	again, err := svc.ImportFont(FontRequest{ID: sum.ID, Path: mana})
	if err != nil || again.ID == res.ID {
		t.Fatalf("again = %+v, %v", again, err)
	}
	// Undo takes the assignment, the entry and the file away
	if _, err := svc.Apply(ApplyRequest{ID: sum.ID, Ops: res.Inverse}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sum.ID, filepath.FromSlash(file))); !os.IsNotExist(err) {
		t.Errorf("the font file is still there: %v", err)
	}
}

func TestImportFontRefusesWhatIsNotAFont(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	notFont := filepath.Join(t.TempDir(), "a.ttf")
	os.WriteFile(notFont, []byte("not a font at all"), 0o644)
	text := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(text, []byte("x"), 0o644)
	for name, req := range map[string]FontRequest{
		"a file that is not a font": {ID: sum.ID, Path: notFont},
		"a file with another type":  {ID: sum.ID, Path: text},
		"a file that is not there":  {ID: sum.ID, Path: filepath.Join(t.TempDir(), "x.otf")},
	} {
		if _, err := svc.ImportFont(req); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	// A role that does not exist as a name is refused with the draft left alone
	if _, err := svc.ImportFont(FontRequest{ID: sum.ID, Path: mana, Assign: []string{"Bad Role"}}); err == nil {
		t.Error("a bad role name was accepted")
	}
	if v, _ := svc.Assets(sum.ID); len(v.Fonts) != 0 {
		t.Errorf("a refused import left fonts: %+v", v.Fonts)
	}
}

func TestContractAtLeast(t *testing.T) {
	for _, tt := range []struct {
		contract string
		want     bool
	}{{"1.1", true}, {"1.2", true}, {"2.0", true}, {"1.0", false}, {"0.9", false}, {"", false}, {"x.y", false}} {
		if got := contractAtLeast(tt.contract, 1, 1); got != tt.want {
			t.Errorf("contractAtLeast(%q) = %v", tt.contract, got)
		}
	}
}

func TestMarksAreKeptWithTheDraft(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	if err := svc.SetMarks(MarksRequest{ID: sum.ID, Locked: []string{"normal/layer:background", "", "normal/layer:background"}, SnapTo: []string{"normal/text:title"}}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Open(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Meta.Locked) != 1 || v.Meta.Locked[0] != "normal/layer:background" || len(v.Meta.SnapTo) != 1 {
		t.Errorf("meta = %+v", v.Meta)
	}
	// They sit beside the test card, and setting one does not lose the other
	if err := svc.SetTestCard(TestCardRequest{ID: sum.ID, Card: nil, Face: 1}); err != nil {
		t.Fatal(err)
	}
	v, _ = svc.Open(sum.ID)
	if len(v.Meta.Locked) != 1 || v.Meta.Face != 1 {
		t.Errorf("meta after a test card = %+v", v.Meta)
	}
	if err := svc.SetMarks(MarksRequest{ID: "nope"}); err == nil {
		t.Error("marks for a draft that is not there")
	}
	if err := svc.SetMarks(MarksRequest{ID: sum.ID, Locked: make([]string, maxMarks+1)}); err == nil {
		t.Error("too many marks were kept")
	}
}

func TestThumbsDrawImagesSmallAndAreKeptUntilTheyChange(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Frame", DPI: 100})
	out, err := svc.Thumbs(ThumbRequest{ID: sum.ID, Files: []string{"parts/background/any.png", "parts/none.png"}, Edge: 64})
	if err != nil {
		t.Fatal(err)
	}
	url := out["parts/background/any.png"]
	if !strings.HasPrefix(url, "data:image/png;base64,") || len(out) != 1 {
		t.Fatalf("thumbs = %v", out)
	}
	cfg, err := png.DecodeConfig(strings.NewReader(decodeDataURL(t, url)))
	if err != nil || cfg.Width > 64 || cfg.Height > 64 || cfg.Width != 47 {
		t.Errorf("thumb is %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	// Replacing the image changes the thumbnail
	if _, err := svc.ImportImage(ImageRequest{ID: sum.ID, Part: "background", Material: "any", Path: writePNG(t, 272, 370, color.NRGBA{R: 255, A: 255})}); err != nil {
		t.Fatal(err)
	}
	next, _ := svc.Thumbs(ThumbRequest{ID: sum.ID, Files: []string{"parts/background/any.png"}, Edge: 64})
	if next["parts/background/any.png"] == url {
		t.Error("the thumbnail did not change with the image")
	}
	for _, bad := range []string{"../x.png", "template.json", "/a.png"} {
		if _, err := svc.Thumbs(ThumbRequest{ID: sum.ID, Files: []string{bad}}); err == nil {
			t.Errorf("a thumbnail of %q was made", bad)
		}
	}
	if _, err := svc.Thumbs(ThumbRequest{ID: sum.ID, Files: make([]string, maxThumbFiles+1)}); err == nil {
		t.Error("too many thumbnails at once")
	}
}

func decodeDataURL(t *testing.T, url string) string {
	t.Helper()
	_, b64, ok := strings.Cut(url, ",")
	if !ok {
		t.Fatal("not a data URL")
	}
	raw, err := base64Decode(b64)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestShrinkAveragesAndKeepsTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	out := shrink(src, 4)
	if b := out.Bounds(); b.Dx() != 4 || b.Dy() != 2 {
		t.Fatalf("size = %v", b)
	}
	if c := out.At(0, 0).(color.NRGBA); c.R != 200 || c.A != 255 {
		t.Errorf("an opaque pixel = %+v", c)
	}
	if c := out.At(3, 1).(color.NRGBA); c.A != 0 {
		t.Errorf("a transparent pixel = %+v", c)
	}
	if shrink(src, 20) != image.Image(src) {
		t.Error("an image under the edge was changed")
	}
}

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
