package cardlist

import (
	"slices"
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func fireIce() *card.Data {
	return &card.Data{
		Name: "Fire // Ice", Layout: "split", Artist: "A & B",
		Faces: []card.Face{
			{Name: "Fire", ManaCost: "{1}{R}", TypeLine: "Instant", OracleText: "Fire deals 2 damage.", Colors: []card.Color{card.Red}},
			{Name: "Ice", ManaCost: "{1}{U}", TypeLine: "Instant", OracleText: "Tap target permanent.", FlavorText: "Cold."},
		},
	}
}

func TestEditsReadSplitHalves(t *testing.T) {
	e := editsOf(fireIce())
	if e.Half1Name != "Fire" || e.Half2Name != "Ice" || e.Half2Flavor != "Cold." {
		t.Errorf("halves read as %+v", e)
	}
	// A half with no colors of its own reads the ones its cost shows
	if e.Half1Colors != "R" || e.Half2Colors != "U" {
		t.Errorf("half colors %q and %q, want R and U", e.Half1Colors, e.Half2Colors)
	}
}

func TestApplyEditsToSplitHalves(t *testing.T) {
	base := fireIce()
	e := editsOf(base)
	e.Half1Name, e.Half2Oracle, e.Half2Colors = "Blaze", "Draw two cards.", "UB"
	got := ApplyEdits(base, e)

	if got.Faces[0].Name != "Blaze" || got.Faces[1].OracleText != "Draw two cards." {
		t.Errorf("edited faces are %+v", got.Faces)
	}
	if !slices.Equal(got.Faces[1].Colors, []card.Color{card.Blue, card.Black}) {
		t.Errorf("half colors = %v", got.Faces[1].Colors)
	}
	if base.Faces[0].Name != "Fire" || base.Faces[1].OracleText != "Tap target permanent." {
		t.Error("the base card was changed")
	}
	if got.Half(0).Name != "Blaze" {
		t.Error("the render reads the edited half")
	}
}

func TestApplyEditsLeavesFacesAlone(t *testing.T) {
	// A card that is not split keeps its faces, whatever the half fields hold
	dfc := &card.Data{Name: "Delver", Layout: "transform", Faces: []card.Face{{Name: "Delver"}, {Name: "Aberration"}}}
	e := editsOf(dfc)
	e.Half1Name = "ignored"
	if got := ApplyEdits(dfc, e); got.Faces[0].Name != "Delver" {
		t.Errorf("a transform card's faces changed: %+v", got.Faces)
	}

	// A split card edited by a client that sends no half fields keeps its halves
	e = editsOf(fireIce())
	e.Half1Name, e.Half1ManaCost, e.Half1Colors, e.Half1TypeLine, e.Half1Oracle, e.Half1Flavor = "", "", "", "", "", ""
	if got := ApplyEdits(fireIce(), e); got.Faces[0].Name != "Fire" {
		t.Errorf("an empty half replaced the fetched one: %+v", got.Faces[0])
	}
}

func TestOverlayNamesAHalfField(t *testing.T) {
	got := Overlay(fireIce(), map[string]string{"half2Name": "Frost"})
	if got.Faces[1].Name != "Frost" || got.Faces[0].Name != "Fire" {
		t.Errorf("overlay changed %+v", got.Faces)
	}
}
