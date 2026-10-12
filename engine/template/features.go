package template

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// ResolveFeatures returns the value of every feature the template declares, from the
// values a caller chose. A value that is missing or does not fit its feature takes
// the feature's default, a number is held to its range and snapped to its step, and
// a name the template does not declare is dropped. Conditions and opacities read
// the result, so they never see a value the template did not offer
func (t *Template) ResolveFeatures(chosen frame.Features) frame.Features {
	out := make(frame.Features, len(t.m.Features))
	for name, f := range t.m.Features {
		if v, ok := fit(f, chosen[name]); ok {
			out[name] = v
		} else {
			out[name] = defaultValue(f)
		}
	}
	return out
}

// fit reports the value as the feature holds it, and false when it is not one the
// feature can hold
func fit(f manifest.Feature, v any) (any, bool) {
	switch f.Type {
	case "bool":
		b, ok := v.(bool)
		return b, ok
	case "choice":
		s, ok := v.(string)
		return s, ok && slices.Contains(f.Values, s)
	case "range":
		n, ok := number(v)
		if !ok || math.IsNaN(n) {
			return nil, false
		}
		return snap(f, n), true
	}
	return nil, false
}

// number reads any numeric type a caller may hold a value in, such as one decoded
// from JSON or typed in a program
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		x, err := n.Float64()
		return x, err == nil
	}
	return 0, false
}

// snap holds a number to the range and moves it to the nearest step from the minimum
func snap(f manifest.Feature, n float64) float64 {
	n = max(f.Min, min(f.Max, n))
	if f.Step > 0 {
		n = f.Min + math.Round((n-f.Min)/f.Step)*f.Step
		n = math.Round(n*1e9) / 1e9
	}
	return max(f.Min, min(f.Max, n))
}

// defaultValue is a feature's declared default, or false, the first value, or the
// minimum when it declares none
func defaultValue(f manifest.Feature) any {
	switch f.Type {
	case "bool":
		var b bool
		_ = json.Unmarshal(f.Default, &b)
		return b
	case "choice":
		var s string
		if json.Unmarshal(f.Default, &s) == nil && slices.Contains(f.Values, s) {
			return s
		}
		if len(f.Values) > 0 {
			return f.Values[0]
		}
		return ""
	}
	n := f.Min
	if len(f.Default) > 0 {
		if err := json.Unmarshal(f.Default, &n); err != nil {
			n = f.Min
		}
	}
	return snap(f, n)
}
