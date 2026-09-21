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

func TestCopyrightDefaultAndOverride(t *testing.T) {
	if got := (&Template{}).copyright(); got != defaultCopyright {
		t.Errorf("empty Copyright = %q, want the default %q", got, defaultCopyright)
	}
	custom := "© 2024 Example"
	if got := (&Template{Copyright: custom}).copyright(); got != custom {
		t.Errorf("set Copyright = %q, want %q", got, custom)
	}
}
