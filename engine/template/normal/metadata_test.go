package normal

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func TestCollectorLine(t *testing.T) {
	cases := []struct {
		name string
		data card.Data
		want string
	}{
		{"rarity then padded number", card.Data{CollectorNumber: "220", Rarity: "uncommon"}, "U 0220"},
		{"mythic", card.Data{CollectorNumber: "1", Rarity: "mythic"}, "M 0001"},
		{"no rarity keeps padded number", card.Data{CollectorNumber: "88"}, "0088"},
		{"non-numeric number is left as is", card.Data{CollectorNumber: "177a", Rarity: "rare"}, "R 177a"},
		{"no number is empty", card.Data{Rarity: "rare"}, ""},
	}
	for _, tc := range cases {
		if got := collectorLine(&tc.data); got != tc.want {
			t.Errorf("%s: collectorLine = %q, want %q", tc.name, got, tc.want)
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
		if got := rarityLetter(in); got != want {
			t.Errorf("rarityLetter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetLine(t *testing.T) {
	cases := []struct {
		name string
		data card.Data
		want string
	}{
		{"code and language", card.Data{SetCode: "a25", Language: "en"}, "A25 • EN"},
		{"missing language defaults to english", card.Data{SetCode: "neo"}, "NEO • EN"},
		{"no set is empty", card.Data{Language: "en"}, ""},
	}
	for _, tc := range cases {
		if got := setLine(&tc.data); got != tc.want {
			t.Errorf("%s: setLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCopyrightCarriesThePrintingsYear(t *testing.T) {
	cases := []struct {
		name string
		tpl  Template
		data card.Data
		want string
	}{
		{
			name: "the year sits between the marks and the holder",
			data: card.Data{ReleasedAt: "2017-11-17"},
			want: "™ & © 2017 Wizards of the Coast",
		},
		{
			// A printing with no date prints the line without a year rather
			// than the year the render happens to run in
			name: "no date drops the year",
			data: card.Data{},
			want: "™ & © Wizards of the Coast",
		},
		{
			name: "a date that is not a year drops it too",
			data: card.Data{ReleasedAt: "soon"},
			want: "™ & © Wizards of the Coast",
		},
		{
			name: "a set Copyright stands in whole",
			tpl:  Template{Copyright: "© 2024 Example"},
			data: card.Data{ReleasedAt: "2017-11-17"},
			want: "© 2024 Example",
		},
	}
	for _, tc := range cases {
		if got := tc.tpl.copyright(&tc.data); got != tc.want {
			t.Errorf("%s: copyright = %q, want %q", tc.name, got, tc.want)
		}
	}
}
