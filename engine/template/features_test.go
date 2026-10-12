package template

import (
	"encoding/json"
	"testing"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

func TestResolveFeatures(t *testing.T) {
	tm := &Template{m: &manifest.Manifest{Features: map[string]manifest.Feature{
		"on":    {Type: "bool", Default: json.RawMessage(`true`)},
		"off":   {Type: "bool"},
		"style": {Type: "choice", Values: []string{"light", "dark"}, Default: json.RawMessage(`"dark"`)},
		"first": {Type: "choice", Values: []string{"a", "b"}},
		"mix":   {Type: "range", Min: 0, Max: 1, Step: 0.05, Default: json.RawMessage(`0.5`)},
		"floor": {Type: "range", Min: 2, Max: 10, Step: 2},
	}}}
	tests := []struct {
		name   string
		chosen frame.Features
		want   frame.Features
	}{
		{"nothing chosen takes defaults", nil,
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 0.5, "floor": 2.0}},
		{"chosen values are kept", frame.Features{"on": false, "off": true, "style": "light", "first": "b", "mix": 0.25, "floor": 6.0},
			frame.Features{"on": false, "off": true, "style": "light", "first": "b", "mix": 0.25, "floor": 6.0}},
		{"wrong types take defaults", frame.Features{"on": "yes", "style": 3, "mix": "high", "floor": true},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 0.5, "floor": 2.0}},
		{"a choice outside its values takes the default", frame.Features{"style": "neon"},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 0.5, "floor": 2.0}},
		{"a range is held to its ends", frame.Features{"mix": 7.0, "floor": -3.0},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 1.0, "floor": 2.0}},
		{"a range snaps to its step", frame.Features{"mix": 0.33, "floor": 7.1},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 0.35, "floor": 8.0}},
		{"other number types are read", frame.Features{"mix": 1, "floor": json.Number("4")},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 1.0, "floor": 4.0}},
		{"an undeclared name is dropped", frame.Features{"ghost": true},
			frame.Features{"on": true, "off": false, "style": "dark", "first": "a", "mix": 0.5, "floor": 2.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tm.ResolveFeatures(tt.chosen)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, w := range tt.want {
				if got[k] != w {
					t.Errorf("%s = %v (%T), want %v (%T)", k, got[k], got[k], w, w)
				}
			}
		})
	}
}

func TestResolveFeaturesRejectsNaN(t *testing.T) {
	tm := &Template{m: &manifest.Manifest{Features: map[string]manifest.Feature{"mix": {Type: "range", Min: 0, Max: 1, Step: 0.1}}}}
	nan := 0.0
	nan /= nan
	if got := tm.ResolveFeatures(frame.Features{"mix": nan})["mix"]; got != 0.0 {
		t.Errorf("NaN resolved to %v, want the minimum", got)
	}
}
