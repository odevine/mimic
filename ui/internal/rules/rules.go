// Package rules is the rule engine behind global overrides. A rule says
// when to act, as conditions on a card's fields that must all hold, and what to
// do, as fields to set or clear. Rules apply to every render in the order they
// are listed, beneath any edit the user made to a card or a list row
package rules

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// The operators a condition takes. All text comparisons ignore case, and
// matches reads its value as a regular expression
const (
	OpContains    = "contains"
	OpNotContains = "notContains"
	OpEquals      = "equals"
	OpNotEquals   = "notEquals"
	OpStartsWith  = "startsWith"
	OpEndsWith    = "endsWith"
	OpMatches     = "matches"
	OpEmpty       = "empty"
	OpNotEmpty    = "notEmpty"
)

// The operators an action takes
const (
	ActionSet   = "set"
	ActionClear = "clear"
)

// Limits that keep a rule list readable and a stored file small
const (
	MaxRules   = 100
	MaxParts   = 10
	MaxValue   = 4000
	maxNameLen = 80
)

// Condition tests one field of a card. empty and notEmpty ignore Value
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value,omitempty"`
}

// Action changes one field. Set writes Value and clear empties the field
type Action struct {
	Op    string `json:"op"`
	Field string `json:"field"`
	Value string `json:"value,omitempty"`
}

// Rule applies its actions to every card that meets all its conditions. A rule
// with no conditions applies to every card. Disabled is how a rule is switched
// off, so a rule stored before the setting existed is on
type Rule struct {
	ID       string      `json:"id"`
	Name     string      `json:"name,omitempty"`
	Disabled bool        `json:"disabled,omitempty"`
	When     []Condition `json:"when"`
	Then     []Action    `json:"then"`
}

var conditionOps = []string{OpContains, OpNotContains, OpEquals, OpNotEquals, OpStartsWith, OpEndsWith, OpMatches, OpEmpty, OpNotEmpty}

// Validate checks a rule list is one the engine can apply, and says what is wrong
// with the first rule that is not
func Validate(rules []Rule) error {
	if len(rules) > MaxRules {
		return fmt.Errorf("there can be at most %d rules", MaxRules)
	}
	fields := cardlist.FieldNames()
	ids := map[string]bool{}
	for i, r := range rules {
		at := fmt.Sprintf("rule %d", i+1)
		if r.ID == "" || ids[r.ID] {
			return fmt.Errorf("%s needs its own id", at)
		}
		ids[r.ID] = true
		if len(r.Name) > maxNameLen {
			return fmt.Errorf("%s has a name longer than %d characters", at, maxNameLen)
		}
		if len(r.When) > MaxParts || len(r.Then) > MaxParts {
			return fmt.Errorf("%s has more than %d conditions or actions", at, MaxParts)
		}
		if len(r.Then) == 0 {
			return fmt.Errorf("%s does nothing, so give it an action", at)
		}
		for _, c := range r.When {
			if !slices.Contains(fields, c.Field) {
				return fmt.Errorf("%s tests %q, which is not a card field", at, c.Field)
			}
			if !slices.Contains(conditionOps, c.Op) {
				return fmt.Errorf("%s tests %q with %q, which is not a comparison", at, c.Field, c.Op)
			}
			if len(c.Value) > MaxValue {
				return fmt.Errorf("%s has a value longer than %d characters", at, MaxValue)
			}
			if c.Op == OpMatches {
				if _, err := regexp.Compile("(?i)" + c.Value); err != nil {
					return fmt.Errorf("%s has a pattern that does not parse: %v", at, err)
				}
			}
		}
		for _, a := range r.Then {
			if a.Op != ActionSet && a.Op != ActionClear {
				return fmt.Errorf("%s has the action %q, which is not set or clear", at, a.Op)
			}
			if !slices.Contains(fields, a.Field) {
				return fmt.Errorf("%s changes %q, which is not a card field", at, a.Field)
			}
			if len(a.Value) > MaxValue {
				return fmt.Errorf("%s has a value longer than %d characters", at, MaxValue)
			}
		}
	}
	return nil
}

// holds reports whether one condition is true of the field values
func (c Condition) holds(values map[string]string) bool {
	have := values[c.Field]
	want := c.Value
	if cardlist.IsColorField(c.Field) {
		have, want = cardlist.NormalizeColors(have), cardlist.NormalizeColors(want)
	}
	switch c.Op {
	case OpEmpty:
		return strings.TrimSpace(have) == ""
	case OpNotEmpty:
		return strings.TrimSpace(have) != ""
	case OpMatches:
		re, err := regexp.Compile("(?i)" + c.Value)
		return err == nil && re.MatchString(have)
	}
	have, want = strings.ToLower(have), strings.ToLower(want)
	switch c.Op {
	case OpContains:
		return strings.Contains(have, want)
	case OpNotContains:
		return !strings.Contains(have, want)
	case OpEquals:
		return have == want
	case OpNotEquals:
		return have != want
	case OpStartsWith:
		return strings.HasPrefix(have, want)
	case OpEndsWith:
		return strings.HasSuffix(have, want)
	}
	return false
}

// Applies reports whether the rule is on and every condition holds for values
func (r Rule) Applies(values map[string]string) bool {
	if r.Disabled {
		return false
	}
	for _, c := range r.When {
		if !c.holds(values) {
			return false
		}
	}
	return true
}

// Result is what a rule list did to one card: the fields it changed, with the
// value each now has, and the rules that applied
type Result struct {
	Fields  map[string]string `json:"fields"`
	Applied []string          `json:"applied"`
}

// Apply runs the rules in order over a card's field values and returns what
// changed. A later rule sees the effect of an earlier one. A field in locked was
// set by the user, so no rule touches it. Only fields whose value ended up
// different from where they started are returned, which keeps a rule that
// changes nothing from marking anything edited
func Apply(rules []Rule, values map[string]string, locked map[string]bool) Result {
	work := make(map[string]string, len(values))
	for k, v := range values {
		work[k] = v
	}
	var applied []string
	for _, r := range rules {
		if !r.Applies(work) {
			continue
		}
		touched := false
		for _, a := range r.Then {
			if locked[a.Field] {
				continue
			}
			to := ""
			if a.Op == ActionSet {
				to = a.Value
			}
			if cardlist.IsColorField(a.Field) {
				to = cardlist.NormalizeColors(to)
			}
			if work[a.Field] != to {
				work[a.Field] = to
				touched = true
			}
		}
		if touched {
			applied = append(applied, r.ID)
		}
	}
	out := Result{Fields: map[string]string{}, Applied: applied}
	for k, v := range work {
		if v != values[k] {
			out.Fields[k] = v
		}
	}
	return out
}

// ApplyToCard applies the rules to a card that carries the given edits of its own
// fields, as a list row does. The edited fields are locked, and the result holds
// only what the rules changed, ready to merge under the edits
func ApplyToCard(rules []Rule, base *card.Data, edits map[string]string) Result {
	locked := make(map[string]bool, len(edits))
	for k := range edits {
		locked[k] = true
	}
	return Apply(rules, cardlist.FieldValues(cardlist.Overlay(base, edits)), locked)
}

// Merge returns the user's edits with the rule's changes underneath: a field the
// user set keeps their value
func Merge(edits, ruled map[string]string) map[string]string {
	if len(ruled) == 0 {
		return edits
	}
	out := make(map[string]string, len(edits)+len(ruled))
	for k, v := range ruled {
		out[k] = v
	}
	for k, v := range edits {
		out[k] = v
	}
	return out
}

// FieldValuesOf is a card's field values with the given fields laid over it, which
// is what a condition reads for a list row
func FieldValuesOf(base *card.Data, fields map[string]string) map[string]string {
	return cardlist.FieldValues(cardlist.Overlay(base, fields))
}
