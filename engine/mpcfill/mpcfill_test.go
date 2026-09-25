package mpcfill

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func face(name string) Face { return Face{Name: name} }

// writeImages stands in for rendering, writing a few bytes to every planned file
func writeImages(t *testing.T, dir string, l *Layout) {
	t.Helper()
	files := []string{l.Cardback}
	for _, c := range l.Cards {
		files = append(files, c.Front, c.Back)
	}
	for _, f := range files {
		if f == "" {
			continue
		}
		name := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStem(t *testing.T) {
	cases := map[string]string{
		"Lightning Bolt":           "Lightning Bolt",
		"Fire // Ice":              "Fire Ice",
		"Island (Unsanctioned)":    "Island Unsanctioned",
		"{EN} Goblin [NSFW]":       "EN Goblin NSFW",
		"Urza's Saga":              "Urza's Saga",
		"Mishra’s Factory":         "Mishra's Factory",
		"Half-Elf":                 "Half-Elf",
		"Æther Vial":               "AEther Vial",
		"Jötun Grunt":              "Jotun Grunt",
		"日本語":                      "Card",
		"  a.b.c  ":                "a b c",
		"!hidden":                  "hidden",
		"card@id":                  "card id",
		"123":                      "Card",
		"":                         "Card",
		"---":                      "Card",
		"Séance":                   "Seance",
		"b:Cardback":               "b Cardback",
		"Tab\tand\nnewline  runs ": "Tab and newline runs",
	}
	for in, want := range cases {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q want %q", in, got, want)
		}
	}
	if got := stem(strings.Repeat("ab", 200)); len(got) != maxStem {
		t.Errorf("long stem has length %d want %d", len(got), maxStem)
	}
	a := strings.Repeat("a", maxStem-2)
	if got := stem(a + " b"); got != a+" b" {
		t.Errorf("a stem of exactly maxStem was cut: %q", got)
	}
	if got := stem(a + "a b"); got != a+"a" {
		t.Errorf("a separator crossing maxStem kept: %q", got)
	}
}

// FuzzStem checks the properties the website's search relies on: at least one
// letter, and nothing it strips or reinterprets
func FuzzStem(f *testing.F) {
	for _, s := range []string{"Fire // Ice", "Island (Unsanctioned)", "{EN} x", "a.b", "", "Æ"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		s := stem(name)
		if s == "" || len(s) > maxStem {
			t.Fatalf("stem(%q) = %q has bad length", name, s)
		}
		if strings.TrimSpace(s) != s || strings.Contains(s, "  ") {
			t.Fatalf("stem(%q) = %q has stray whitespace", name, s)
		}
		if !strings.ContainsAny(strings.ToLower(s), "abcdefghijklmnopqrstuvwxyz") {
			t.Fatalf("stem(%q) = %q has no letter", name, s)
		}
		for _, r := range s {
			ok := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' ||
				r == ' ' || r == '-' || r == '\''
			if !ok {
				t.Fatalf("stem(%q) = %q contains %q", name, s, r)
			}
		}
	})
}

func TestDPI(t *testing.T) {
	for h, want := range map[int]int{1110: 300, 1111: 300, 4440: 1200, 5568: MaxDPI, 5569: MaxDPI + 10} {
		if got := DPI(h); got != want {
			t.Errorf("DPI(%d) = %d want %d", h, got, want)
		}
	}
}

func TestNamesDeduplicateCaseInsensitively(t *testing.T) {
	n := names{}
	var got []string
	for _, name := range []string{"Goblin", "goblin", "GOBLIN!", "Goblin 2", "Con", "com1", "LPT9", "Console"} {
		got = append(got, n.entry(FrontsDir, face(name), 0, nil).path)
	}
	want := []string{
		"fronts/Goblin.png", "fronts/goblin 2.png", "fronts/GOBLIN 3.png", "fronts/Goblin 2 2.png",
		"fronts/Con 2.png", "fronts/com1 2.png", "fronts/LPT9 2.png", "fronts/Console.png",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d path = %q want %q", i, got[i], want[i])
		}
	}
}

func TestSlotsAreConsecutive(t *testing.T) {
	back := face("Transformed")
	l, err := Project{
		Cardback: &Face{Name: "Back"},
		Cards: []Card{
			{Front: face("A"), Quantity: 2},
			{Front: face("B"), Back: &back},
			{Front: face("C"), Quantity: -4},
			{Front: face("D"), Quantity: 3},
		},
	}.layout()
	if err != nil {
		t.Fatal(err)
	}
	if l.quantity != 7 {
		t.Errorf("quantity = %d want 7", l.quantity)
	}
	want := [][]int{{0, 1}, {2}, {3}, {4, 5, 6}}
	for i, e := range l.fronts {
		if !slices.Equal(e.slots, want[i]) {
			t.Errorf("front %d slots = %v want %v", i, e.slots, want[i])
		}
	}
	if len(l.backs) != 1 || !slices.Equal(l.backs[0].slots, []int{2}) {
		t.Errorf("backs = %+v want one back in slot 2", l.backs)
	}
	if l.stock != S30 {
		t.Errorf("zero stock resolved to %q want %q", l.stock, S30)
	}
}

func TestValidation(t *testing.T) {
	cb := face("Back")
	cases := []struct {
		name string
		p    Project
		want error
	}{
		{"no cards", Project{Cardback: &cb}, ErrNoCards},
		{"unknown stock", Project{Stock: "S30", Cardback: &cb, Cards: []Card{{Front: face("A")}}}, ErrStock},
		{"foil plastic", Project{Stock: P10, Foil: true, Cardback: &cb, Cards: []Card{{Front: face("A")}}}, ErrFoil},
		{"no cardback", Project{Cards: []Card{{Front: face("A")}}}, ErrNoCardback},
		{"over max", Project{Cardback: &cb, Cards: []Card{{Front: face("A"), Quantity: MaxProjectSize + 1}}}, ErrTooManyCards},
		{"huge quantity", Project{Cardback: &cb, Cards: []Card{{Front: face("A"), Quantity: 1 << 40}}}, ErrTooManyCards},
		{"over max across cards", Project{Cardback: &cb, Cards: []Card{
			{Front: face("A"), Quantity: MaxProjectSize}, {Front: face("B")},
		}}, ErrTooManyCards},
	}
	for _, c := range cases {
		l, err := Plan(c.p)
		if !errors.Is(err, c.want) || l != nil {
			t.Errorf("%s: Plan = %v, %v want nil, %v", c.name, l, err, c.want)
		}
	}
}

func TestProjectLimits(t *testing.T) {
	cb := face("Back")
	if _, err := (Project{Cardback: &cb, Cards: []Card{{Front: face("A"), Quantity: MaxProjectSize}}}).layout(); err != nil {
		t.Errorf("a project of exactly MaxProjectSize should be valid: %v", err)
	}
	for _, s := range []Stock{S27, S30, S33, M31} {
		if _, err := (Project{Stock: s, Foil: true, Cardback: &cb, Cards: []Card{{Front: face("A")}}}).layout(); err != nil {
			t.Errorf("foil %s should be valid: %v", s, err)
		}
	}
	back := face("B back")
	if _, err := (Project{Cards: []Card{{Front: face("B"), Back: &back}}}).layout(); err != nil {
		t.Errorf("an all double-faced project needs no cardback: %v", err)
	}
	if _, err := (Project{Cardback: &cb, Cards: []Card{{Front: face("B"), Back: &back}}}).layout(); err != nil {
		t.Errorf("an all double-faced project may still have a cardback: %v", err)
	}
}

func TestFrontAndBackNamesAreIndependent(t *testing.T) {
	back := face("Delver")
	l, err := Project{Cards: []Card{{Front: face("Delver"), Back: &back}}}.layout()
	if err != nil {
		t.Fatal(err)
	}
	if l.fronts[0].path != "fronts/Delver.png" || l.backs[0].path != "backs/Delver.png" {
		t.Errorf("paths = %q, %q", l.fronts[0].path, l.backs[0].path)
	}
}

func TestPlan(t *testing.T) {
	back := face("Delver // Aberration")
	l, err := Plan(Project{
		Cardback: &Face{Name: "Classic Back"},
		Cards: []Card{
			{Front: face("Island"), Quantity: 3},
			{Front: face("Delver of Secrets"), Back: &back},
			{Front: face("island")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []CardFiles{
		{Front: "fronts/Island.png"},
		{Front: "fronts/Delver of Secrets.png", Back: "backs/Delver Aberration.png"},
		{Front: "fronts/island 2.png"},
	}
	if !slices.Equal(l.Cards, want) {
		t.Errorf("cards = %+v want %+v", l.Cards, want)
	}
	if l.Cardback != "cardback/Classic Back.png" {
		t.Errorf("cardback = %q", l.Cardback)
	}
}

func TestWriteOrder(t *testing.T) {
	dir := t.TempDir()
	back := face("Delver // Aberration")
	l, err := Plan(Project{
		Stock:    S33,
		Foil:     true,
		Cardback: &Face{Name: "Classic Back"},
		Cards: []Card{
			{Front: face("Island (Unsanctioned)"), Quantity: 3},
			{Front: face("Delver of Secrets"), Back: &back},
			{Front: face("island unsanctioned")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeImages(t, dir, l)
	if err := WriteOrder(dir, l); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, OrderFile))
	if err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<order>
    <details>
        <quantity>5</quantity>
        <stock>(S33) Superior Smooth</stock>
        <foil>true</foil>
    </details>
    <fronts>
        <card>
            <id>./fronts/Island Unsanctioned.png</id>
            <sourceType>Local File</sourceType>
            <slots>0,1,2</slots>
            <name>Island Unsanctioned.png</name>
            <query>island unsanctioned</query>
        </card>
        <card>
            <id>./fronts/Delver of Secrets.png</id>
            <sourceType>Local File</sourceType>
            <slots>3</slots>
            <name>Delver of Secrets.png</name>
            <query>delver of secrets</query>
        </card>
        <card>
            <id>./fronts/island unsanctioned 2.png</id>
            <sourceType>Local File</sourceType>
            <slots>4</slots>
            <name>island unsanctioned 2.png</name>
            <query>island unsanctioned 2</query>
        </card>
    </fronts>
    <backs>
        <card>
            <id>./backs/Delver Aberration.png</id>
            <sourceType>Local File</sourceType>
            <slots>3</slots>
            <name>Delver Aberration.png</name>
            <query>delver aberration</query>
        </card>
    </backs>
    <cardback>./cardback/Classic Back.png</cardback>
</order>
`
	if string(got) != want {
		t.Errorf("order file:\n%s\nwant:\n%s", got, want)
	}

	// Every id must name a file relative to dir, as the desktop tool reads it
	// after changing into dir
	var o xmlOrder
	if err := xml.Unmarshal(got, &o); err != nil {
		t.Fatal(err)
	}
	ids := []string{o.Cardback}
	for _, c := range append(o.Fronts.Cards, o.Backs.Cards...) {
		ids = append(ids, c.ID)
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(id))); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

func TestWriteOrderNeedsEveryImage(t *testing.T) {
	back := face("B back")
	l, err := Plan(Project{Cardback: &Face{Name: "Back"}, Cards: []Card{{Front: face("A")}, {Front: face("B"), Back: &back}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{l.Cards[0].Front, l.Cards[1].Back, l.Cardback} {
		dir := t.TempDir()
		writeImages(t, dir, l)
		name := filepath.Join(dir, filepath.FromSlash(missing))
		os.Remove(name)
		if err := WriteOrder(dir, l); !errors.Is(err, ErrMissingImage) || !strings.Contains(err.Error(), missing) {
			t.Errorf("without %s: err = %v want ErrMissingImage naming it", missing, err)
		}
		// The desktop tool treats an empty file as missing too
		os.WriteFile(name, nil, 0o644)
		if err := WriteOrder(dir, l); !errors.Is(err, ErrMissingImage) {
			t.Errorf("with %s empty: err = %v want ErrMissingImage", missing, err)
		}
		if _, err := os.Stat(filepath.Join(dir, OrderFile)); !os.IsNotExist(err) {
			t.Errorf("without %s: order file written anyway", missing)
		}
	}
}

func TestWriteOrderOmitsUnusedElements(t *testing.T) {
	order := func(p Project) string {
		t.Helper()
		l, err := Plan(p)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		writeImages(t, dir, l)
		if err := WriteOrder(dir, l); err != nil {
			t.Fatal(err)
		}
		out, _ := os.ReadFile(filepath.Join(dir, OrderFile))
		return string(out)
	}

	back := face("B back")
	out := order(Project{Cards: []Card{{Front: face("B"), Back: &back}}})
	if strings.Contains(out, "<cardback") {
		t.Errorf("order without a cardback has a cardback element:\n%s", out)
	}

	cb := face("Back")
	out = order(Project{Cardback: &cb, Cards: []Card{{Front: face("A")}}})
	if strings.Contains(out, "<backs") {
		t.Errorf("order without double-faced cards has a backs element:\n%s", out)
	}
}
