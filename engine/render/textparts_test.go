package render

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/engine/template"
)

// TestTextPartsArtistPrefix covers the choice between the modern frame's nib
// icon and an old-bordered frame's literal "Illus. " prefix: a box with no
// Prefix keeps drawing the nib when one is available, and a box with a
// Prefix draws the literal text instead and carries no symbol
func TestTextPartsArtistPrefix(t *testing.T) {
	fr := fonts.NewResolver(nil, nil, nil)
	d := &card.Data{Artist: "Kev Walker"}
	syms := symbols{artist: mana.ArtistNib{}}

	t.Run("no prefix draws the nib", func(t *testing.T) {
		parts := textParts("artist", template.TextBoxSpec{}, d, card.Taken{}, syms, fr, "")
		if len(parts) != 1 {
			t.Fatalf("got %d parts, want 1", len(parts))
		}
		if parts[0].Text != "{"+mana.NibCode+"}Kev Walker" {
			t.Errorf("text = %q, want the nib code before the name", parts[0].Text)
		}
		if parts[0].Sym == nil {
			t.Error("Sym is nil, want the artist nib renderer")
		}
	})

	t.Run("prefix draws literal text and no nib", func(t *testing.T) {
		box := template.TextBoxSpec{Prefix: "Illus. "}
		parts := textParts("artist", box, d, card.Taken{}, syms, fr, "")
		if len(parts) != 1 {
			t.Fatalf("got %d parts, want 1", len(parts))
		}
		if parts[0].Text != "Illus. Kev Walker" {
			t.Errorf("text = %q, want %q", parts[0].Text, "Illus. Kev Walker")
		}
		if parts[0].Sym != nil {
			t.Error("Sym is set, want nil when a Prefix labels the credit")
		}
	})

	t.Run("prefix with no artist draws nothing", func(t *testing.T) {
		box := template.TextBoxSpec{Prefix: "Illus. "}
		parts := textParts("artist", box, &card.Data{}, card.Taken{}, syms, fr, "")
		if len(parts) != 0 {
			t.Errorf("got %d parts, want 0 for an empty artist credit", len(parts))
		}
	})
}
