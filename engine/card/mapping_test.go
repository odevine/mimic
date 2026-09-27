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

func TestFromScryfallJSONFrontFaceByLayout(t *testing.T) {
	// Trimmed from the live API. A double-faced card's top level joins both
	// faces and has no art, while a split, adventure, or flip card prints on one
	// face and its top level already describes it
	cases := []struct {
		name, raw         string
		wantName, wantArt string
	}{
		{
			"modal double-faced",
			`{"name":"Emeria's Call // Emeria, Shattered Skyclave","layout":"modal_dfc","type_line":"Sorcery // Land",
			  "card_faces":[{"name":"Emeria's Call","type_line":"Sorcery","mana_cost":"{4}{W}{W}{W}","image_uris":{"art_crop":"front.jpg"}},
			                {"name":"Emeria, Shattered Skyclave","type_line":"Land","image_uris":{"art_crop":"back.jpg"}}]}`,
			"Emeria's Call", "front.jpg",
		},
		{
			"split",
			`{"name":"Fire // Ice","layout":"split","type_line":"Instant // Instant","mana_cost":"{1}{R} // {1}{U}","image_uris":{"art_crop":"whole.jpg"},
			  "card_faces":[{"name":"Fire","type_line":"Instant"},{"name":"Ice","type_line":"Instant"}]}`,
			"Fire // Ice", "whole.jpg",
		},
		{
			"adventure",
			`{"name":"Bonecrusher Giant // Stomp","layout":"adventure","type_line":"Creature — Giant // Instant — Adventure","power":"4","image_uris":{"art_crop":"whole.jpg"},
			  "card_faces":[{"name":"Bonecrusher Giant","type_line":"Creature — Giant"},{"name":"Stomp","type_line":"Instant — Adventure"}]}`,
			"Bonecrusher Giant // Stomp", "whole.jpg",
		},
		{
			"flip",
			`{"name":"Nezumi Shortfang // Stabwhisker the Odious","layout":"flip","type_line":"Creature — Rat Rogue // Legendary Creature — Rat Shaman","image_uris":{"art_crop":"whole.jpg"},
			  "card_faces":[{"name":"Nezumi Shortfang","type_line":"Creature — Rat Rogue"},{"name":"Stabwhisker the Odious","type_line":"Legendary Creature — Rat Shaman"}]}`,
			"Nezumi Shortfang // Stabwhisker the Odious", "whole.jpg",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := FromScryfallJSON([]byte(c.raw))
			if err != nil {
				t.Fatal(err)
			}
			if d.Name != c.wantName || d.ArtworkURL != c.wantArt {
				t.Errorf("name %q art %q, want %q %q", d.Name, d.ArtworkURL, c.wantName, c.wantArt)
			}
			if len(d.Faces) != 2 {
				t.Errorf("kept %d faces, want 2", len(d.Faces))
			}
		})
	}
}
func TestFromScryfallJSONColorIndicator(t *testing.T) {
	// A single-faced card carries its indicator at the top level, and a
	// double-faced card's top level takes its front face's
	d, err := FromScryfallJSON([]byte(`{"name":"Dryad Arbor","layout":"normal","type_line":"Land Creature — Forest Dryad","color_indicator":["G"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ColorIndicator) != 1 || d.ColorIndicator[0] != Green {
		t.Errorf("indicator %v, want [G]", d.ColorIndicator)
	}
	d, err = FromScryfallJSON([]byte(`{"name":"A // B","layout":"transform","frame_effects":["legendary","sunmoondfc"],
	  "card_faces":[{"name":"A","type_line":"Creature","color_indicator":["R"]},{"name":"B","type_line":"Creature","color_indicator":["G","R"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ColorIndicator) != 1 || d.ColorIndicator[0] != Red {
		t.Errorf("front indicator %v, want [R]", d.ColorIndicator)
	}
	if got := d.Face(1).ColorIndicator; len(got) != 2 {
		t.Errorf("back indicator %v, want [G R]", got)
	}
}
