package card

import "testing"

func TestCollectorLine(t *testing.T) {
	cases := []struct {
		name string
		data Data
		want string
	}{
		{"rarity then padded number", Data{CollectorNumber: "220", Rarity: "uncommon"}, "U 0220"},
		{"mythic", Data{CollectorNumber: "1", Rarity: "mythic"}, "M 0001"},
		{"no rarity keeps padded number", Data{CollectorNumber: "88"}, "0088"},
		{"non-numeric number is left as is", Data{CollectorNumber: "177a", Rarity: "rare"}, "R 177a"},
		{"no number is empty", Data{Rarity: "rare"}, ""},
	}
	for _, tc := range cases {
		if got := CollectorLine(&tc.data); got != tc.want {
			t.Errorf("%s: CollectorLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRarityLetter(t *testing.T) {
	cases := map[string]string{
		"common": "C", "uncommon": "U", "rare": "R", "mythic": "M",
		"special": "S", "bonus": "B", "": "",
		"COMMON": "C", // case insensitive
		"promo":  "P", // unknown rarity takes its first letter
	}
	for in, want := range cases {
		if got := RarityLetter(in); got != want {
			t.Errorf("RarityLetter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetLine(t *testing.T) {
	cases := []struct {
		name string
		data Data
		want string
	}{
		{"code and language", Data{SetCode: "a25", Language: "en"}, "A25 • EN"},
		{"missing language defaults to english", Data{SetCode: "neo"}, "NEO • EN"},
		{"no set is empty", Data{Language: "en"}, ""},
	}
	for _, tc := range cases {
		if got := SetLine(&tc.data); got != tc.want {
			t.Errorf("%s: SetLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCopyrightLineCarriesThePrintingsYear(t *testing.T) {
	cases := []struct {
		name     string
		override string
		data     Data
		want     string
	}{
		{
			name: "the year sits between the marks and the holder",
			data: Data{ReleasedAt: "2017-11-17"},
			want: "™ & © 2017 Wizards of the Coast",
		},
		{
			// A printing with no date prints the line without a year rather
			// than the year the render happens to run in
			name: "no date drops the year",
			data: Data{},
			want: "™ & © Wizards of the Coast",
		},
		{
			name: "a date that is not a year drops it too",
			data: Data{ReleasedAt: "soon"},
			want: "™ & © Wizards of the Coast",
		},
		{
			name:     "an override stands in whole",
			override: "© 2024 Example",
			data:     Data{ReleasedAt: "2017-11-17"},
			want:     "© 2024 Example",
		},
	}
	for _, tc := range cases {
		if got := CopyrightLine(tc.override, &tc.data); got != tc.want {
			t.Errorf("%s: CopyrightLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFlipsidePT(t *testing.T) {
	militia := &Data{Layout: "transform", Faces: []Face{
		{Name: "Hanweir Militia Captain", Power: "2", Toughness: "3"},
		{Name: "Westvale Cult Leader", Power: "*", Toughness: "*"},
	}}
	if got := TextFor("flipside_pt", militia); got != "*/*" {
		t.Errorf("front flipside_pt = %q, want */*", got)
	}
	if got := TextFor("flipside_pt", militia.Face(1)); got != "" {
		t.Errorf("back flipside_pt = %q, want empty", got)
	}
	azcanta := &Data{Layout: "transform", Faces: []Face{{Name: "Search for Azcanta"}, {Name: "Azcanta, the Sunken Ruin"}}}
	if got := TextFor("flipside_pt", azcanta); got != "" {
		t.Errorf("flipside_pt with a noncreature back = %q, want empty", got)
	}
	adventure := &Data{Layout: "adventure", Faces: []Face{{Name: "Giant", Power: "4", Toughness: "3"}, {Name: "Stomp", Power: "1", Toughness: "1"}}}
	if got := TextFor("flipside_pt", adventure); got != "" {
		t.Errorf("flipside_pt on a one-faced layout = %q, want empty", got)
	}
}
