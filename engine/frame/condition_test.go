package frame

import (
	"slices"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func TestEveryConditionHoldsExactlyWhenItsFactDoes(t *testing.T) {
	cases := []struct {
		name string
		on   Keys
		off  Keys
	}{
		{"land", Keys{Land: true}, Keys{}},
		{"nonland", Keys{}, Keys{Land: true}},
		{"legendary", Keys{Legendary: true}, Keys{}},
		{"nonlegendary", Keys{}, Keys{Legendary: true}},
		{"creature", Keys{Creature: true}, Keys{}},
		{"nyx", Keys{Nyx: true}, Keys{}},
		{"nonnyx", Keys{}, Keys{Nyx: true}},
		{"companion", Keys{Companion: true}, Keys{}},
		{"color_indicator", Keys{Indicator: Named("u")}, Keys{}},
		{"front", Keys{Front: true}, Keys{}},
		{"back", Keys{Back: true}, Keys{}},
		{"fuse", Keys{Fuse: true}, Keys{}},
		{"set_symbol", Keys{SetSymbol: true}, Keys{}},
		{"icon_left", Keys{Front: true, TransformIcon: Named("sunmoondfc")}, Keys{}},
		{"icon_right", Keys{Back: true, TransformIcon: Named(triangleIcon)}, Keys{Back: true, TransformIcon: Named("sunmoondfc")}},
	}
	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.name] = true
		cond, err := Compile(c.name)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.name, err)
		}
		if !cond.Met(c.on, nil) {
			t.Errorf("%q should hold for %+v", c.name, c.on)
		}
		if cond.Met(c.off, nil) {
			t.Errorf("%q should not hold for %+v", c.name, c.off)
		}
	}
	for _, name := range Conditions() {
		if !covered[name] {
			t.Errorf("condition %q is in the vocabulary and has no case here", name)
		}
	}
}

func TestEmptyConditionAlwaysHolds(t *testing.T) {
	for _, expr := range []string{"", "  "} {
		c, err := Compile(expr)
		if err != nil || !c.Met(Keys{}, nil) {
			t.Errorf("Compile(%q) = %v, holds %v", expr, err, err == nil && c.Met(Keys{}, nil))
		}
	}
	if !(Condition{}).Met(Keys{}, nil) {
		t.Error("the zero Condition should always hold")
	}
}

func TestCompoundConditionNeedsEveryTerm(t *testing.T) {
	k := Keys{Land: true, Back: true}
	cases := map[string]bool{
		"land,back":        true,
		"back, land":       true,
		" land , back ":    true,
		"land,back,nyx":    false,
		"nonland,back":     false,
		"land,back,nonnyx": true,
	}
	for expr, want := range cases {
		c, err := Compile(expr)
		if err != nil {
			t.Fatalf("Compile(%q): %v", expr, err)
		}
		if got := c.Met(k, nil); got != want {
			t.Errorf("%q = %v, want %v", expr, got, want)
		}
	}
}

func TestCompileRejectsWhatTheEngineDoesNotHave(t *testing.T) {
	bad := []string{
		"nyxx",
		"Land",
		"hollow_crown",
		"fullart",
		"pt_dark",
		"clipped",
		"divider",
		"land,",
		",land",
		"land,,back",
		"feature:",
		"feature:x<",
		"feature:x<big",
		"feature:x!y",
		"feature: x",
	}
	for _, expr := range bad {
		if _, err := Compile(expr); err == nil {
			t.Errorf("Compile(%q) succeeded, want an error", expr)
		}
	}
	_, err := Compile("land,nyxx")
	if err == nil || !strings.Contains(err.Error(), `"nyxx"`) || !strings.Contains(err.Error(), `"land,nyxx"`) {
		t.Errorf("the error should name the term and the expression, got %v", err)
	}
}

func TestFeatureTerms(t *testing.T) {
	f := Features{"textless": true, "ptStyle": "dark", "texture": 0.75, "shadow": false}
	cases := []struct {
		expr string
		want bool
	}{
		{"feature:textless", true},
		{"feature:shadow", false},
		{"feature:missing", false},
		{"feature:ptStyle", false},
		{"feature:ptStyle=dark", true},
		{"feature:ptStyle=light", false},
		{"feature:textless=true", true},
		{"feature:textless=false", false},
		{"feature:shadow=false", true},
		{"feature:texture=0.75", true},
		{"feature:texture=1", false},
		{"feature:texture>0.5", true},
		{"feature:texture>=0.75", true},
		{"feature:texture<0.75", false},
		{"feature:texture<=0.75", true},
		{"feature:texture > 0.5", true},
		{"feature:ptStyle>1", false},
		{"feature:textless>1", false},
		{"feature:texture=dark", false},
		{"feature:textless,feature:ptStyle=dark", true},
		{"feature:textless,feature:ptStyle=light", false},
		{"land,feature:textless", false},
	}
	for _, c := range cases {
		cond, err := Compile(c.expr)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.expr, err)
		}
		if got := cond.Met(Keys{}, f); got != c.want {
			t.Errorf("%q = %v, want %v", c.expr, got, c.want)
		}
	}
	cond, _ := Compile("feature:textless")
	if cond.Met(Keys{}, nil) {
		t.Error("a feature with no values chosen should not hold")
	}
}

func TestConditionStringIsTheExpressionAsWritten(t *testing.T) {
	c, err := Compile("back, land")
	if err != nil || c.String() != "back, land" {
		t.Errorf("String = %q, %v", c.String(), err)
	}
}

func TestConditionsListsTheVocabularySorted(t *testing.T) {
	got := Conditions()
	if !slices.IsSorted(got) || !slices.Contains(got, "nyx") || slices.Contains(got, "pt_dark") {
		t.Errorf("Conditions() = %v", got)
	}
}

func TestDeriveReadsTheCompanionFrame(t *testing.T) {
	cases := []struct {
		effects []string
		want    bool
	}{
		{nil, false},
		{[]string{"legendary"}, false},
		{[]string{"companion"}, true},
		{[]string{"legendary", "companion"}, true},
	}
	for _, c := range cases {
		k := Derive(&card.Data{TypeLine: "Creature", FrameEffects: c.effects})
		if k.Companion != c.want || k.ConditionMet("companion") != c.want {
			t.Errorf("effects %v: companion = %v, want %v", c.effects, k.Companion, c.want)
		}
	}
}

func TestNyxConditionsFollowTheType(t *testing.T) {
	god := Derive(&card.Data{TypeLine: "Legendary Enchantment Creature — God"})
	angel := Derive(&card.Data{TypeLine: "Creature — Angel"})
	if !god.ConditionMet("nyx") || god.ConditionMet("nonnyx") {
		t.Error("an enchantment creature should be nyx and not nonnyx")
	}
	if angel.ConditionMet("nyx") || !angel.ConditionMet("nonnyx") {
		t.Error("an angel should be nonnyx and not nyx")
	}
}

func TestFeatureRefsListsEachReferenceInOrder(t *testing.T) {
	c, err := Compile("land,feature:textless,feature:ptStyle=dark,feature:texture>=0.5")
	if err != nil {
		t.Fatal(err)
	}
	want := []FeatureRef{
		{Name: "textless"},
		{Name: "ptStyle", Op: "=", Value: "dark"},
		{Name: "texture", Op: ">=", Value: "0.5", Number: 0.5, IsNumber: true},
	}
	got := c.FeatureRefs()
	if len(got) != len(want) {
		t.Fatalf("FeatureRefs = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ref %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if refs := (Condition{}).FeatureRefs(); refs != nil {
		t.Errorf("an empty condition refers to %+v", refs)
	}
}
