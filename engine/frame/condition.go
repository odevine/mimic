package frame

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Features holds the value chosen for each of a template's features, by name. A
// value is a bool, a string, or a float64
type Features map[string]any

// Condition is a compiled condition expression. It is a comma-separated list of
// terms that all have to hold, so the empty expression always holds. A term is a
// name from the engine's vocabulary, such as "land" or "front", or a reference
// to a feature: "feature:textless" for a feature that is on,
// "feature:ptStyle=dark" for a feature with that value, and
// "feature:texture>0.5" for a number, which compares with =, <, <=, > or >=.
// A count the card decides compares with a number the same way, as "rows>3".
// The zero Condition always holds
type Condition struct {
	src   string
	terms []term
}

type term struct {
	name    string // a name from the vocabulary or of a count, or empty for a feature term
	count   bool   // name is a count, which compares with op and num
	feature string
	op      string // "" for a feature that must be on, otherwise =, <, <=, > or >=
	value   string
	num     float64
	isNum   bool
}

// vocabulary is every condition a card decides. Each one is a fact about the card
// or the frame being drawn, never a choice, which a feature makes
var vocabulary = map[string]func(Keys) bool{
	"land":            func(k Keys) bool { return k.Land },
	"nonland":         func(k Keys) bool { return !k.Land },
	"legendary":       func(k Keys) bool { return k.Legendary },
	"nonlegendary":    func(k Keys) bool { return !k.Legendary },
	"creature":        func(k Keys) bool { return k.Creature },
	"nyx":             func(k Keys) bool { return k.Nyx },
	"nonnyx":          func(k Keys) bool { return !k.Nyx },
	"companion":       func(k Keys) bool { return k.Companion },
	"color_indicator": func(k Keys) bool { return !k.Indicator.IsNone() },
	"front":           func(k Keys) bool { return k.Front },
	"back":            func(k Keys) bool { return k.Back },
	"fuse":            func(k Keys) bool { return k.Fuse },
	"set_symbol":      func(k Keys) bool { return k.SetSymbol },
	"icon_left":       func(k Keys) bool { return !k.TransformIcon.IsNone() && !k.iconRight() },
	"icon_right":      func(k Keys) bool { return k.iconRight() },
	"loyalty":         func(k Keys) bool { return k.Loyalty },
	"chaos":           func(k Keys) bool { return k.Chaos },
	"snow":            func(k Keys) bool { return k.Snow },
	"nonsnow":         func(k Keys) bool { return !k.Snow },
}

// counts are the numbers a card decides that a condition compares, such as
// "rows>3" for a planeswalker with four abilities or more
var counts = map[string]func(Keys) int{
	"rows": func(k Keys) int { return k.Rows },
}

// Conditions lists the names a condition can use, sorted, for a message that
// tells an author what is available. A count is listed by its name and compares
// with a number
func Conditions() []string {
	out := make([]string, 0, len(vocabulary)+len(counts))
	for n := range vocabulary {
		out = append(out, n)
	}
	for n := range counts {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Compile reads a condition expression. It reports a name the engine does not
// have, so a misspelt condition is an error where it is written and not a layer
// that never draws
func Compile(expr string) (Condition, error) {
	c := Condition{src: expr}
	if strings.TrimSpace(expr) == "" {
		return c, nil
	}
	for _, raw := range strings.Split(expr, ",") {
		t, err := compileTerm(strings.TrimSpace(raw))
		if err != nil {
			return Condition{}, fmt.Errorf("condition %q: %w", expr, err)
		}
		c.terms = append(c.terms, t)
	}
	return c, nil
}

func compileTerm(s string) (term, error) {
	if s == "" {
		return term{}, fmt.Errorf("empty term")
	}
	rest, isFeature := strings.CutPrefix(s, "feature:")
	if !isFeature {
		if _, ok := vocabulary[s]; ok {
			return term{name: s}, nil
		}
		return compileCount(s)
	}
	end := 0
	for end < len(rest) && isNameChar(rest[end]) {
		end++
	}
	t := term{feature: rest[:end]}
	if t.feature == "" {
		return term{}, fmt.Errorf("feature reference has no name")
	}
	if rest = strings.TrimLeft(rest[end:], " "); rest == "" {
		return t, nil
	}
	for _, op := range []string{"<=", ">=", "<", ">", "="} {
		if v, ok := strings.CutPrefix(rest, op); ok {
			t.op, t.value = op, strings.TrimSpace(v)
			break
		}
	}
	if t.op == "" {
		return term{}, fmt.Errorf("feature %q is followed by %q, want a comparison", t.feature, rest)
	}
	if t.value == "" {
		return term{}, fmt.Errorf("feature %q compares with %s but gives no value", t.feature, t.op)
	}
	if n, err := strconv.ParseFloat(t.value, 64); err == nil {
		t.num, t.isNum = n, true
	}
	if t.op != "=" && !t.isNum {
		return term{}, fmt.Errorf("feature %q compares with %s but %q is not a number", t.feature, t.op, t.value)
	}
	return t, nil
}

// compileCount reads a comparison of a count with a number, such as "rows>=4"
func compileCount(s string) (term, error) {
	end := 0
	for end < len(s) && isNameChar(s[end]) {
		end++
	}
	name := s[:end]
	if _, ok := counts[name]; !ok {
		return term{}, fmt.Errorf("unknown condition %q", s)
	}
	rest := strings.TrimSpace(s[end:])
	t := term{name: name, count: true}
	for _, op := range []string{"<=", ">=", "<", ">", "="} {
		if v, ok := strings.CutPrefix(rest, op); ok {
			t.op, t.value = op, strings.TrimSpace(v)
			break
		}
	}
	n, err := strconv.ParseFloat(t.value, 64)
	if t.op == "" || err != nil {
		return term{}, fmt.Errorf("%q is a count, which compares with a number as in %q", name, name+">3")
	}
	t.num, t.isNum = n, true
	return t, nil
}

func isNameChar(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// FeatureRef is a reference to a template feature inside a condition. Op is empty
// for a bare reference, which asks whether the feature is on, and otherwise one
// of =, <, <=, > or >= with the value written after it. Number is the value read
// as a number when it is one
type FeatureRef struct {
	Name     string
	Op       string
	Value    string
	Number   float64
	IsNumber bool
}

// FeatureRefs lists the features a condition refers to, in the order written, so
// a manifest can be checked against the features it declares
func (c Condition) FeatureRefs() []FeatureRef {
	var out []FeatureRef
	for _, t := range c.terms {
		if t.feature != "" {
			out = append(out, FeatureRef{Name: t.feature, Op: t.op, Value: t.value, Number: t.num, IsNumber: t.isNum})
		}
	}
	return out
}

// String is the expression as it was written
func (c Condition) String() string { return c.src }

// Met reports whether every term holds for the card these keys were read from and
// the feature values chosen. A feature with no value never holds
func (c Condition) Met(k Keys, f Features) bool {
	for _, t := range c.terms {
		if !t.met(k, f) {
			return false
		}
	}
	return true
}

func (t term) met(k Keys, f Features) bool {
	if t.count {
		return compare(float64(counts[t.name](k)), t.op, t.num)
	}
	if t.feature == "" {
		return vocabulary[t.name](k)
	}
	v, ok := f[t.feature]
	if !ok {
		return false
	}
	if t.op == "" {
		on, isBool := v.(bool)
		return isBool && on
	}
	switch v := v.(type) {
	case bool:
		return t.op == "=" && t.value == strconv.FormatBool(v)
	case string:
		return t.op == "=" && t.value == v
	case float64:
		return t.isNum && compare(v, t.op, t.num)
	}
	return false
}

// compare reports whether v stands in the relation op to n
func compare(v float64, op string, n float64) bool {
	switch op {
	case "=":
		return v == n
	case "<":
		return v < n
	case "<=":
		return v <= n
	case ">":
		return v > n
	case ">=":
		return v >= n
	}
	return false
}

// ConditionMet reports whether an expression holds for the card these keys were
// read from. An expression that does not compile does not hold, and a caller that
// evaluates one repeatedly can Compile it once. Feature values are not known
// here, so a feature term does not hold
func (k Keys) ConditionMet(expr string) bool {
	c, err := Compile(expr)
	return err == nil && c.Met(k, nil)
}
