package card

import (
	"slices"
	"testing"
)

func fireIce() *Data {
	return &Data{
		Name: "Fire // Ice", Layout: "split", Colors: []Color{Blue, Red},
		Faces: []Face{
			{Name: "Fire", ManaCost: "{1}{R}", TypeLine: "Instant", OracleText: "Fire deals 2 damage."},
			{Name: "Ice", ManaCost: "{1}{U}", TypeLine: "Instant", OracleText: "Tap target permanent."},
		},
	}
}

func TestHalfReadsEachFace(t *testing.T) {
	d := fireIce()
	fire, ice := d.Half(0), d.Half(1)
	if fire.Name != "Fire" || ice.Name != "Ice" {
		t.Errorf("halves are %q and %q", fire.Name, ice.Name)
	}
	if fire == d {
		t.Error("half 0 should read the first face, not return the whole card")
	}
	if !slices.Equal(fire.Colors, []Color{Red}) || !slices.Equal(ice.Colors, []Color{Blue}) {
		t.Errorf("half colors %v and %v, want red and blue from the mana costs", fire.Colors, ice.Colors)
	}
	if d.Half(2) != d || d.Half(-1) != d {
		t.Error("a half out of range should be the card itself")
	}
}

func TestHalfKeepsColorsScryfallGives(t *testing.T) {
	d := fireIce()
	d.Faces[0].Colors = []Color{Green}
	if got := d.Half(0).Colors; !slices.Equal(got, []Color{Green}) {
		t.Errorf("colors = %v, want the face's own", got)
	}
}

func TestFuse(t *testing.T) {
	const reminder = "Fuse (You may cast one or both halves of this card from your hand.)"
	d := &Data{
		Layout: "split", Keywords: []string{"Fuse"},
		Faces: []Face{
			{Name: "Wear", ManaCost: "{1}{R}", OracleText: "Destroy target artifact.\n" + reminder},
			{Name: "Tear", ManaCost: "{W}", OracleText: "Destroy target enchantment.\n" + reminder},
		},
	}
	if !d.HasFuse() || d.FuseText() != reminder {
		t.Errorf("HasFuse = %v, FuseText = %q", d.HasFuse(), d.FuseText())
	}
	if got := d.Half(0).OracleText; got != "Destroy target artifact." {
		t.Errorf("half text = %q, want the reminder left out", got)
	}
	if TextFor("fuse", d) != reminder {
		t.Error("the fuse box should print the reminder once")
	}

	// A card with no keywords is still fuse by its reminder line
	d.Keywords = nil
	if !d.HasFuse() {
		t.Error("a face's Fuse reminder should mark a card with no keywords")
	}
	if fireIce().HasFuse() || fireIce().FuseText() != "" {
		t.Error("an ordinary split card is not a fuse card")
	}
}

func TestColorsFromManaCost(t *testing.T) {
	cases := map[string][]Color{
		"{1}{R}":          {Red},
		"{X}{G}{G}":       {Green},
		"{G/W}{G/W}":      {White, Green},
		"{2/W}{U}":        {White, Blue},
		"{B/P}":           {Black},
		"{3}{C}{S}":       nil,
		"":                nil,
		"{W}{U}{B}{R}{G}": {White, Blue, Black, Red, Green},
	}
	for cost, want := range cases {
		if got := ColorsFromManaCost(cost); !slices.Equal(got, want) {
			t.Errorf("ColorsFromManaCost(%q) = %v, want %v", cost, got, want)
		}
	}
}

func TestKeywordsMapFromScryfall(t *testing.T) {
	d, err := FromScryfallJSON([]byte(`{"name":"Wear // Tear","layout":"split","keywords":["Fuse"],"card_faces":[{"name":"Wear"},{"name":"Tear"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Keywords, []string{"Fuse"}) {
		t.Errorf("Keywords = %v", d.Keywords)
	}
}

func TestSplitFacesTakeColorsFromManaCost(t *testing.T) {
	d, err := FromScryfallJSON([]byte(`{"name":"Fire // Ice","layout":"split","card_faces":[{"name":"Fire","mana_cost":"{1}{R}"},{"name":"Ice","mana_cost":"{1}{U}"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Faces[0].Colors, []Color{Red}) || !slices.Equal(d.Faces[1].Colors, []Color{Blue}) {
		t.Errorf("face colors %v and %v, want red and blue", d.Faces[0].Colors, d.Faces[1].Colors)
	}
}
