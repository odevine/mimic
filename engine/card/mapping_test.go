package card

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromScryfallJSON(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "lightning_bolt.json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromScryfallJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Lightning Bolt" || d.ManaCost != "{R}" || len(d.Colors) != 1 || d.Colors[0] != Red {
		t.Errorf("mapped %+v", d)
	}

	// A double-faced card maps through the same front-face fallback a lookup uses
	raw, err = os.ReadFile(filepath.Join("testdata", "delver.json"))
	if err != nil {
		t.Fatal(err)
	}
	if d, err = FromScryfallJSON(raw); err != nil || d.Name != "Delver of Secrets" {
		t.Errorf("mapped %+v, %v", d, err)
	}
	if d.Layout != "transform" || len(d.Faces) != 2 {
		t.Fatalf("layout %q with %d faces, want transform with 2", d.Layout, len(d.Faces))
	}
	back := d.Face(1)
	if back.Name != "Insectile Aberration" || back.Power != "3" || back.SetCode != "isd" || back.ArtworkURL == d.ArtworkURL {
		t.Errorf("back face %+v", back)
	}
	if d.Face(0) != d || d.Face(5) != d {
		t.Error("face 0 and an out of range face should be the card itself")
	}

	if _, err := FromScryfallJSON([]byte("{not json")); err == nil {
		t.Error("malformed JSON should fail")
	}
}

func TestArtCropURL(t *testing.T) {
	got := ArtCropURL("02645651-cd55-4bd0-8a4d-fa257270a0e0")
	want := "https://cards.scryfall.io/art_crop/front/0/2/02645651-cd55-4bd0-8a4d-fa257270a0e0.jpg"
	if got != want {
		t.Errorf("ArtCropURL = %q, want %q", got, want)
	}
	if ArtCropURL("a") != "" {
		t.Error("a one-character id should give no URL")
	}
}
