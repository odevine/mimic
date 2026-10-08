package rules

import (
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func creature() *card.Data {
	return &card.Data{Name: "Llanowar Elves", TypeLine: "Creature — Elf Druid", ManaCost: "{G}", Power: "1", Toughness: "1", OracleText: "{T}: Add {G}.", Colors: []card.Color{"G"}}
}

func rule(id string, when []Condition, then ...Action) Rule {
	return Rule{ID: id, When: when, Then: then}
}

func set(field, value string) Action { return Action{Op: ActionSet, Field: field, Value: value} }

func TestConditionsIgnoreCaseAndAllMustHold(t *testing.T) {
	values := map[string]string{"typeLine": "Creature — Elf Druid", "power": "1", "flavor": ""}
	for _, c := range []struct {
		cond Condition
		want bool
	}{
		{Condition{"typeLine", OpContains, "creature"}, true},
		{Condition{"typeLine", OpContains, "goblin"}, false},
		{Condition{"typeLine", OpNotContains, "goblin"}, true},
		{Condition{"typeLine", OpEquals, "creature — elf druid"}, true},
		{Condition{"typeLine", OpNotEquals, "creature"}, true},
		{Condition{"typeLine", OpStartsWith, "CREATURE"}, true},
		{Condition{"typeLine", OpEndsWith, "druid"}, true},
		{Condition{"typeLine", OpMatches, `^creature .* (elf|goblin)`}, true},
		{Condition{"power", OpMatches, `^\d+$`}, true},
		{Condition{"flavor", OpEmpty, ""}, true},
		{Condition{"power", OpNotEmpty, ""}, true},
		{Condition{"power", OpEmpty, ""}, false},
	} {
		if got := c.cond.holds(values); got != c.want {
			t.Errorf("%+v = %v, want %v", c.cond, got, c.want)
		}
	}
	both := rule("a", []Condition{{"typeLine", OpContains, "creature"}, {"power", OpEquals, "4"}}, set("flavor", "x"))
	if both.Applies(values) {
		t.Error("a rule applied with one of two conditions false")
	}
	if !rule("b", nil, set("flavor", "x")).Applies(values) {
		t.Error("a rule with no conditions did not apply")
	}
	off := rule("c", nil, set("flavor", "x"))
	off.Disabled = true
	if off.Applies(values) {
		t.Error("a disabled rule applied")
	}
}

func TestColorsCompareAsSets(t *testing.T) {
	values := map[string]string{"colors": "WU"}
	if !(Condition{"colors", OpEquals, "uw"}).holds(values) {
		t.Error("UW did not equal WU")
	}
	if !(Condition{"colors", OpContains, "u"}).holds(values) {
		t.Error("WU did not contain U")
	}
	res := Apply([]Rule{rule("r", nil, set("colors", "gbb"))}, values, nil)
	if res.Fields["colors"] != "BG" {
		t.Errorf("colors set to %q, want BG in order without repeats", res.Fields["colors"])
	}
}

func TestApplyReturnsOnlyWhatChangedAndNamesTheRules(t *testing.T) {
	values := map[string]string{"typeLine": "Creature — Elf", "power": "1", "toughness": "1", "flavor": "old"}
	rules := []Rule{
		rule("big", []Condition{{"typeLine", OpContains, "creature"}}, set("power", "4"), set("toughness", "1")),
		rule("noflavor", nil, Action{Op: ActionClear, Field: "flavor"}),
		rule("never", []Condition{{"typeLine", OpContains, "goblin"}}, set("power", "9")),
	}
	res := Apply(rules, values, nil)
	if len(res.Fields) != 2 || res.Fields["power"] != "4" || res.Fields["flavor"] != "" {
		t.Errorf("fields = %v, want power 4 and flavor cleared, and toughness left alone since it did not change", res.Fields)
	}
	if strings.Join(res.Applied, ",") != "big,noflavor" {
		t.Errorf("applied = %v", res.Applied)
	}
	if values["power"] != "1" {
		t.Error("Apply changed the values it was given")
	}
}

func TestLaterRulesSeeEarlierOnes(t *testing.T) {
	values := map[string]string{"typeLine": "Creature", "power": "1"}
	rules := []Rule{
		rule("one", nil, set("typeLine", "Artifact Creature")),
		rule("two", []Condition{{"typeLine", OpContains, "artifact"}}, set("power", "3")),
	}
	if got := Apply(rules, values, nil).Fields["power"]; got != "3" {
		t.Errorf("power = %q, want the second rule to see the first's type line", got)
	}
}

func TestLockedFieldsKeepTheUsersValue(t *testing.T) {
	values := map[string]string{"power": "7", "toughness": "1"}
	rules := []Rule{rule("r", nil, set("power", "4"), set("toughness", "4"))}
	res := Apply(rules, values, map[string]bool{"power": true})
	if _, touched := res.Fields["power"]; touched || res.Fields["toughness"] != "4" {
		t.Errorf("fields = %v, want only toughness changed", res.Fields)
	}
}

func TestApplyToCardTreatsTheRowsOwnFieldsAsTheUsers(t *testing.T) {
	rules := []Rule{
		rule("pt", []Condition{{"typeLine", OpContains, "elf"}}, set("power", "4"), set("toughness", "4")),
	}
	res := ApplyToCard(rules, creature(), map[string]string{"power": "2"})
	if _, touched := res.Fields["power"]; touched {
		t.Errorf("a rule overwrote the row's own power: %v", res.Fields)
	}
	if res.Fields["toughness"] != "4" {
		t.Errorf("fields = %v, want toughness 4", res.Fields)
	}
	merged := Merge(map[string]string{"power": "2"}, res.Fields)
	if merged["power"] != "2" || merged["toughness"] != "4" {
		t.Errorf("merged = %v", merged)
	}
	if got := Merge(nil, nil); got != nil {
		t.Errorf("merging nothing gave %v", got)
	}
}

func TestRowFieldsAreWhatConditionsSee(t *testing.T) {
	// The row renames the card, and the rule tests the name it will print
	rules := []Rule{rule("r", []Condition{{"name", OpEquals, "Llanowar Warband"}}, set("flavor", "Together."))}
	res := ApplyToCard(rules, creature(), map[string]string{"name": "Llanowar Warband"})
	if res.Fields["flavor"] != "Together." {
		t.Errorf("fields = %v", res.Fields)
	}
}

func TestValidate(t *testing.T) {
	ok := rule("a", []Condition{{"typeLine", OpContains, "elf"}}, set("power", "4"))
	if err := Validate([]Rule{ok}); err != nil {
		t.Fatalf("a good rule was refused: %v", err)
	}
	bad := map[string]Rule{
		"no id":         {When: ok.When, Then: ok.Then},
		"no action":     {ID: "x", When: ok.When},
		"unknown field": rule("x", []Condition{{"nope", OpContains, "a"}}, set("power", "1")),
		"unknown op":    rule("x", []Condition{{"power", "near", "1"}}, set("power", "1")),
		"bad regex":     rule("x", []Condition{{"power", OpMatches, "("}}, set("power", "1")),
		"bad action":    rule("x", nil, Action{Op: "double", Field: "power"}),
		"action field":  rule("x", nil, set("nope", "1")),
		"long value":    rule("x", nil, set("flavor", strings.Repeat("a", MaxValue+1))),
	}
	for name, r := range bad {
		if err := Validate([]Rule{r}); err == nil {
			t.Errorf("%s: the rule was accepted", name)
		}
	}
	if err := Validate([]Rule{ok, ok}); err == nil {
		t.Error("two rules with one id were accepted")
	}
	var many []Rule
	for i := range MaxRules + 1 {
		r := ok
		r.ID = strings.Repeat("a", i+1)
		many = append(many, r)
	}
	if err := Validate(many); err == nil {
		t.Error("too many rules were accepted")
	}
}
