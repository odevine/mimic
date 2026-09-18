package normal

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func TestResolveColorKey(t *testing.T) {
	cases := []struct {
		name string
		data *card.Data
		want string
	}{
		{"mono red", &card.Data{TypeLine: "Instant", ColorIdentity: []card.Color{card.Red}}, "r"},
		{"multicolor", &card.Data{TypeLine: "Creature — Elf", ColorIdentity: []card.Color{card.Green, card.White}}, "gold"},
		{"colorless artifact", &card.Data{TypeLine: "Artifact", ColorIdentity: nil}, "artifact"},
		{"colorless nonartifact", &card.Data{TypeLine: "Creature — Eldrazi", ColorIdentity: nil}, "colorless"},
		{"basic land", &card.Data{TypeLine: "Basic Land — Mountain", ColorIdentity: nil}, "land"},
		{"land beats color identity", &card.Data{TypeLine: "Land", ColorIdentity: []card.Color{card.Blue}}, "land"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveColorKey(c.data); got != c.want {
				t.Errorf("ResolveColorKey = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsLegendary(t *testing.T) {
	if !IsLegendary(&card.Data{TypeLine: "Legendary Creature — God"}) {
		t.Error("expected legendary")
	}
	if IsLegendary(&card.Data{TypeLine: "Creature — Human"}) {
		t.Error("expected nonlegendary")
	}
}

func TestConditionMet(t *testing.T) {
	cases := []struct {
		condition string
		colorKey  string
		legendary bool
		want      bool
	}{
		{"", "r", false, true},
		{"legendary", "r", true, true},
		{"legendary", "r", false, false},
		{"nonlegendary", "r", false, true},
		{"land", "land", false, true},
		{"land", "r", false, false},
		{"nonland", "r", false, true},
		{"nonland", "land", false, false},
		{"r", "r", false, true}, // color key as condition, the escape hatch
		{"r", "u", false, false},
	}
	for _, c := range cases {
		got := conditionMet(c.condition, c.colorKey, c.legendary)
		if got != c.want {
			t.Errorf("conditionMet(%q, %q, %v) = %v, want %v", c.condition, c.colorKey, c.legendary, got, c.want)
		}
	}
}

func TestBlendMode(t *testing.T) {
	if _, err := blendMode("multiply"); err != nil {
		t.Errorf("multiply: %v", err)
	}
	if _, err := blendMode(""); err != nil {
		t.Errorf("empty: %v", err)
	}
	if _, err := blendMode("not-a-mode"); err == nil {
		t.Error("expected error for unknown blend mode")
	}
}
