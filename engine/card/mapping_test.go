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
