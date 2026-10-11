package manifest

import (
	"fmt"
	"strings"
	"testing"
)

type change struct {
	path  string
	value any
}

func TestEachMistakeIsReportedAtItsPath(t *testing.T) {
	const (
		normal = "layouts.normal."
		title  = normal + "textBoxes.title"
	)
	cases := []struct {
		name    string
		changes []change
		path    string
		sev     Severity
		msg     string
	}{
		// the header
		{"a bad id", []change{{"id", "Mimic M15"}}, "id", Error, "is not an id like"},
		{"an id with no owner", []change{{"id", "m15"}}, "id", Error, "is not an id like"},
		{"no name", []change{{"name", ""}}, "name", Error, "no name"},
		{"no description", []change{{"description", gone}}, "description", Warning, "no description"},
		{"a credit with no name", []change{{"credits[0].name", ""}}, "credits[0].name", Error, "no name"},
		{"no layouts", []change{{"layouts", map[string]any{}}}, "layouts", Error, "no layouts"},

		// parts
		{"an unknown slot", []change{{"parts.background.slot", "border"}}, "parts.background.slot", Error, "unknown color slot"},
		{"no images", []change{{"parts.border.variants", map[string]any{}}}, "parts.border.variants", Error, "no images"},
		{"a key that is not a material", []change{{"parts.background.variants.zz", map[string]any{"path": "a.png"}}}, "parts.background.variants.zz", Error, "is not a material"},
		{"a pair out of WUBRG order", []change{{"parts.pinlines.variants.uw", map[string]any{"path": "a.png"}}}, "parts.pinlines.variants.uw", Error, "is not a material"},
		{"a key with capitals", []change{{"parts.background.variants.W", map[string]any{"path": "a.png"}}}, "parts.background.variants.W", Error, "is not an image key"},
		{"a color on a part with no slot", []change{{"parts.border.variants.w", map[string]any{"path": "a.png"}}}, "parts.border.variants.w", Error, "only an \"any\" image"},
		{"an indicator out of order", []change{{"parts.indicator.variants.bu", map[string]any{"path": "a.png"}}}, "parts.indicator.variants.bu", Error, "not a set of colors"},
		{"a transform icon the engine does not select", []change{{"parts.icon.variants.sparkle", map[string]any{"path": "a.png"}}}, "parts.icon.variants.sparkle", Warning, "never draws"},
		{"an absolute path", []change{{"parts.background.variants.w.path", "/etc/a.png"}}, "parts.background.variants.w.path", Error, "is absolute"},
		{"a path that climbs out", []change{{"parts.background.variants.w.path", "a/../../b.png"}}, "parts.background.variants.w.path", Error, "may leave the bundle"},
		{"a path with an empty part", []change{{"parts.background.variants.w.path", "a//b.png"}}, "parts.background.variants.w.path", Error, "may leave the bundle"},
		{"a backslash path", []change{{"parts.background.variants.w.path", "a\\b.png"}}, "parts.background.variants.w.path", Error, "backslash"},
		{"a drive path", []change{{"parts.background.variants.w.path", "C:/a.png"}}, "parts.background.variants.w.path", Error, "backslash, a colon"},
		{"a path that is not a PNG", []change{{"parts.background.variants.w.path", "a.jpg"}}, "parts.background.variants.w.path", Error, "is not a PNG"},
		{"no path", []change{{"parts.background.variants.w.path", ""}}, "parts.background.variants.w.path", Error, "no path"},
		{"a long path", []change{{"parts.background.variants.w.path", strings.Repeat("a", 300) + ".png"}}, "parts.background.variants.w.path", Error, "over the limit"},
		{"a negative origin", []change{{"parts.background.origin.x", -5}}, "parts.background.origin", Error, "outside the canvas"},
		{"a part name with capitals", []change{{"parts.Big", map[string]any{"variants": map[string]any{"any": map[string]any{"path": "a.png"}}}}}, "parts.Big", Error, "is not a part name"},

		// blends
		{"an unknown shape", []change{{"parts.pinlines.blend.shape", "cubic"}}, "parts.pinlines.blend.shape", Error, "smooth or linear"},
		{"an end above one", []change{{"parts.pinlines.blend.end", 1.5}}, "parts.pinlines.blend.end", Error, "outside 0 to 1"},
		{"a seam with no width", []change{{"parts.pinlines.blend.seams.2[0].width", 0}}, "parts.pinlines.blend.seams.2[0].width", Error, "not a width"},
		{"a seam off the canvas", []change{{"parts.pinlines.blend.seams.2[0].center", 99999}}, "parts.pinlines.blend.seams.2[0].center", Error, "off the canvas"},
		{"seams for a run of five", []change{{"parts.pinlines.blend.seams.5", []any{map[string]any{"width": 1}}}}, "parts.pinlines.blend.seams.5", Error, "not a number of colors"},
		{"seams that are not a count", []change{{"parts.pinlines.blend.seams.many", []any{}}}, "parts.pinlines.blend.seams.many", Error, "not a number of colors"},
		{"seams that overlap", []change{{"parts.pinlines.blend.seams.3", []any{map[string]any{"center": 0, "width": 1000}, map[string]any{"center": 100, "width": 100}}}}, "parts.pinlines.blend.seams.3[1]", Error, "before the one before it ends"},
		{"the wrong number of seams", []change{{"parts.pinlines.blend.seams.3", []any{map[string]any{"width": 1}}}}, "parts.pinlines.blend.seams.3", Error, "has 2 seams, and there are 1"},
		{"a blend that does nothing", []change{{"parts.pt_box.blend", map[string]any{"end": 0.9}}}, "parts.pt_box.blend", Warning, "never reads a run"},
		{"a part nothing uses", []change{{"parts.spare", map[string]any{"variants": map[string]any{"any": map[string]any{"path": "a.png"}}}}}, "parts.spare", Warning, "no layout uses"},

		// layouts
		{"a layout that serves nothing", []change{{normal + "serves", []any{}}}, normal + "serves", Error, "serves no shape"},
		{"an unknown role", []change{{normal + "serves[0].role", "sideways"}}, normal + "serves[0].role", Error, "is not a role"},
		{"the unknown kind", []change{{normal + "serves[0].kind", "unknown"}}, normal + "serves[0].kind", Error, "is not a kind"},
		{"two layouts serving one shape", []change{{"layouts.split.serves[0]", map[string]any{"role": "single", "kind": "standard"}}}, "layouts.split.serves[0]", Error, "both serve"},
		{"a canvas with no width", []change{{normal + "canvas.width", 0}}, normal + "canvas", Error, "is not a size"},
		{"a canvas that is too large", []change{{normal + "canvas.height", 20001}}, normal + "canvas", Error, "is not a size"},
		{"a rotation that is not a quarter turn", []change{{normal + "canvas.rotate", 45}}, normal + "canvas.rotate", Error, "is not 0, 90, 180, or 270"},
		{"a negative resolution", []change{{normal + "canvas.dpi", -1}}, normal + "canvas.dpi", Error, "not a resolution"},
		{"an empty stack", []change{{normal + "stack", []any{}}}, normal + "stack", Error, "stack is empty"},
		{"no art window", []change{{normal + "art", []any{}}}, normal + "art", Warning, "no art window"},

		// stack entries
		{"an entry for a part that is not there", []change{{normal + "stack[1].part", "nope"}}, normal + "stack[1].part", Error, `no part is named "nope"`},
		{"an id used twice", []change{{normal + "stack[1].id", "background"}}, normal + "stack[1].id", Error, "used twice"},
		{"an id with a space", []change{{normal + "stack[1].id", "Bad Id"}}, normal + "stack[1].id", Error, "is not an entry id"},
		{"a condition the engine does not have", []change{{normal + "stack[0].condition", "nyxx"}}, normal + "stack[0].condition", Error, `unknown condition "nyxx"`},
		{"a condition from the old vocabulary", []change{{normal + "stack[0].condition", "pt_dark"}}, normal + "stack[0].condition", Error, "unknown condition"},
		{"a feature nobody declared", []change{{normal + "stack[0].condition", "feature:nothing"}}, normal + "stack[0].condition", Error, "does not declare"},
		{"a choice used as a switch", []change{{normal + "stack[0].condition", "feature:ptStyle"}}, normal + "stack[0].condition", Error, "cannot stand alone"},
		{"a bool compared with a word", []change{{normal + "stack[0].condition", "feature:textless=maybe"}}, normal + "stack[0].condition", Error, "true or false"},
		{"a choice compared with a value it lacks", []change{{normal + "stack[0].condition", "feature:ptStyle=neon"}}, normal + "stack[0].condition", Error, "is not a value of the feature"},
		{"a choice that is ordered", []change{{normal + "stack[0].condition", "feature:ptStyle>1"}}, normal + "stack[0].condition", Error, "only a range compares"},
		{"a range compared with a word", []change{{normal + "stack[0].condition", "feature:texture=dark"}}, normal + "stack[0].condition", Error, "compares with a number"},
		{"a condition in a mirror", []change{{normal + "stack[4].mirror.condition", "nyxx"}}, normal + "stack[4].mirror.condition", Error, "unknown condition"},
		{"a mirror with a negative corner", []change{{normal + "stack[4].mirror.x", -1}}, normal + "stack[4].mirror", Error, "negative"},
		{"an unknown mode", []change{{normal + "stack[3].mode", "burn"}}, normal + "stack[3].mode", Error, "unknown blend mode"},
		{"an opacity above one", []change{{normal + "stack[3].opacity", 1.5}}, normal + "stack[3].opacity", Error, "outside 0 to 1"},
		{"an opacity and a feature", []change{{normal + "stack[2].opacity", 0.5}}, normal + "stack[2]", Error, "both set"},
		{"an opacity feature nobody declared", []change{{normal + "stack[2].opacityFeature", "nothing"}}, normal + "stack[2].opacityFeature", Error, "declares no feature"},
		{"an opacity feature that is not a range", []change{{normal + "stack[2].opacityFeature", "ptStyle"}}, normal + "stack[2].opacityFeature", Error, "not a range feature"},
		{"an opacity feature that goes past one", []change{{"features.texture.max", 2}}, normal + "stack[2].opacityFeature", Error, "within 0 to 1"},
		{"a face that does not exist", []change{{normal + "stack[0].face", "third"}}, normal + "stack[0].face", Error, "is not a face"},
		{"a rotation on an entry", []change{{normal + "stack[0].rotate", 45}}, normal + "stack[0].rotate", Error, "is not 0, 90, 180, or 270"},
		{"a placement off the canvas", []change{{"layouts.split.stack[0].at.x", 90000}}, "layouts.split.stack[0].at", Error, "off the canvas"},

		// text boxes
		{"a box with no width", []change{{title + ".rect.width", 0}}, title + ".rect", Error, "is not a size"},
		{"text of no size", []change{{title + ".text.fontSize", 0}}, title + ".text.fontSize", Error, "not a size above 0"},
		{"a floor above the size", []change{{title + ".text.minFontSize", 500}}, title + ".text.minFontSize", Error, "not from 0 to the font size"},
		{"a font role that does not exist", []change{{title + ".text.font", "comic"}}, title + ".text.font", Error, "is not a font role"},
		{"a color that is not hex", []change{{title + ".text.color", "red"}}, title + ".text.color", Error, "#RRGGBB"},
		{"an alignment that does not exist", []change{{title + ".text.align", "middle"}}, title + ".text.align", Error, "is not left, center, or right"},
		{"a vertical alignment that does not exist", []change{{title + ".text.vAlign", "middle"}}, title + ".text.vAlign", Error, "is not top, center, bottom, or baseline"},
		{"negative padding", []change{{title + ".text.paddingX", -3}}, title + ".text", Error, "cannot be negative"},
		{"a shadow that is too opaque", []change{{normal + "textBoxes.mana.text.shadow.opacity", 2}}, normal + "textBoxes.mana.text.shadow", Error, "opacity is from 0 to 1"},
		{"a shadow color that is not hex", []change{{normal + "textBoxes.mana.text.shadow.color", "black"}}, normal + "textBoxes.mana.text.shadow.color", Error, "#RRGGBB"},
		{"a negative gap", []change{{title + ".flow.clearGap", -1}}, title + ".flow.clearGap", Error, "cannot be negative"},
		{"clearing a box that is not there", []change{{title + ".flow.clearOf", "nope"}}, title + ".flow.clearOf", Error, "not another text box"},
		{"clearing itself", []change{{title + ".flow.clearOf", "title"}}, title + ".flow.clearOf", Error, "not another text box"},
		{"avoiding an entry that is not there", []change{{normal + "textBoxes.oracle.flow.avoidLayer", "nope"}}, normal + "textBoxes.oracle.flow.avoidLayer", Error, "no stack entry has the id"},
		{"ducking under an entry that is not there", []change{{normal + "textBoxes.copyright.flow.duck.layer", "nope"}}, normal + "textBoxes.copyright.flow.duck.layer", Error, "no stack entry has the id"},
		{"ducking to a row that is not there", []change{{normal + "textBoxes.copyright.flow.duck.row", "nope"}}, normal + "textBoxes.copyright.flow.duck.row", Error, "not another text box"},
		{"a divider that is not a part", []change{{normal + "textBoxes.oracle.flow.divider", "nope"}}, normal + "textBoxes.oracle.flow.divider", Error, "no part is named"},
		{"a box condition that does not compile", []change{{normal + "textBoxes.pt.condition", "nyxx"}}, normal + "textBoxes.pt.condition", Error, "unknown condition"},
		{"a space that does not exist", []change{{"layouts.split.textBoxes.copyright.space", "input"}}, "layouts.split.textBoxes.copyright.space", Error, "is not empty or"},
		{"a box that draws nothing", []change{{normal + "textBoxes.flavor", map[string]any{"rect": map[string]any{"x": 0, "y": 0, "width": 10, "height": 10}, "text": map[string]any{"fontSize": 10}}}}, normal + "textBoxes.flavor", Warning, "fills no text"},
		{"a box that draws a name with a space", []change{{normal + "textBoxes.pt.draws", "p t"}}, normal + "textBoxes.pt.draws", Error, "not something a box draws"},
		{"a box that draws nothing the engine fills", []change{{normal + "textBoxes.pt.draws", "flavor"}}, normal + "textBoxes.pt", Warning, "draws \"flavor\""},
		{"a box name with a space", []change{{normal + "textBoxes.bad name", map[string]any{"rect": map[string]any{"x": 0, "y": 0, "width": 10, "height": 10}, "text": map[string]any{"fontSize": 10}}}}, normal + "textBoxes.bad name", Error, "is not a text box name"},

		// overrides
		{"an override with no condition", []change{{title + ".when[0].condition", ""}}, title + ".when[0].condition", Error, "no condition"},
		{"an override condition that does not compile", []change{{title + ".when[0].condition", "nyxx"}}, title + ".when[0].condition", Error, "unknown condition"},
		{"an override that names a field a box lacks", []change{{title + ".when[0].colour", 1}}, title + ".when[0]", Error, "unknown field"},
		{"an override that makes a bad box", []change{{title + ".when[0].rect.width", -5}}, title + ".when[0].rect", Error, "is not a size"},
		{"an override inside an override", []change{{title + ".when[0].when", []any{map[string]any{"condition": "land"}}}}, title + ".when[0]", Error, "overrides of its own"},
		{"an override that points at nothing", []change{{title + ".when[0].flow.clearOf", "nope"}}, title + ".when[0].flow.clearOf", Error, "not another text box"},

		// art and symbols
		{"art after an entry that is not there", []change{{normal + "art[0].after", "nope"}}, normal + "art[0].after", Error, "no stack entry has the id"},
		{"an art window with no height", []change{{normal + "art[0].height", 0}}, normal + "art[0]", Error, "is not a size"},
		{"an art window with a face that does not exist", []change{{normal + "art[0].face", "third"}}, normal + "art[0].face", Error, "is not a face"},
		{"a symbol the engine does not draw", []change{{normal + "symbols.rarity", map[string]any{"x": 0, "y": 0, "width": 10, "height": 10}}}, normal + "symbols.rarity", Error, "only \"set\""},
		{"a symbol scaled up", []change{{normal + "symbols.set.scale", 2}}, normal + "symbols.set.scale", Error, "outside 0 to 1"},
		{"a symbol aligned nowhere", []change{{normal + "symbols.set.align", "middle"}}, normal + "symbols.set.align", Error, "is not left, center, or right"},
		{"a symbol condition that does not compile", []change{{normal + "symbols.set.condition", "nyxx"}}, normal + "symbols.set.condition", Error, "unknown condition"},

		// features
		{"a feature name that starts with a digit", []change{{"features.1bad", map[string]any{"type": "bool"}}}, "features.1bad", Error, "is not a feature name"},
		{"a feature type that does not exist", []change{{"features.ptStyle.type", "slider"}}, "features.ptStyle.type", Error, "is not a feature type"},
		{"a bool with values", []change{{"features.textless.values", []any{"a"}}}, "features.textless", Error, "no values or range"},
		{"a choice with no values", []change{{"features.ptStyle.values", []any{}}}, "features.ptStyle.values", Error, "at least one value"},
		{"a choice with a value listed twice", []change{{"features.ptStyle.values", []any{"a", "a"}}}, "features.ptStyle.values[1]", Error, "listed twice"},
		{"a choice with a value that cannot be written in a condition", []change{{"features.ptStyle.values", []any{"a,b"}}}, "features.ptStyle.values[0]", Error, "is not a value"},
		{"a choice default that is not a value", []change{{"features.ptStyle.default", "neon"}}, "features.ptStyle.default", Error, "not one of the values"},
		{"a default of the wrong type", []change{{"features.ptStyle.default", 3}}, "features.ptStyle.default", Error, "does not fit a choice"},
		{"a range that holds nothing", []change{{"features.texture.min", 2}}, "features.texture", Error, "holds nothing"},
		{"a range with no step", []change{{"features.texture.step", 0}}, "features.texture.step", Error, "does not fit"},
		{"a range default outside it", []change{{"features.texture.default", 5}}, "features.texture.default", Error, "outside the range"},
		{"a range with values", []change{{"features.texture.values", []any{"a"}}}, "features.texture.values", Error, "no values"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := edit(t, func(r map[string]any) {
				for _, ch := range c.changes {
					set(t, r, ch.path, ch.value)
				}
			})
			_, ds := Decode(data)
			d, ok := find(ds, c.path)
			// Several diagnostics can share a path, so look for the one that matches
			for _, other := range ds {
				if other.Path == c.path && other.Severity == c.sev && strings.Contains(other.Message, c.msg) {
					d, ok = other, true
					break
				}
			}
			if !ok {
				t.Fatalf("no diagnostic at %q in %v", c.path, ds)
			}
			if d.Severity != c.sev || !strings.Contains(d.Message, c.msg) {
				t.Errorf("at %q: %v, want a %v mentioning %q", c.path, d, c.sev, c.msg)
			}
			if c.sev == Error && !ds.HasErrors() {
				t.Error("an error did not stop the manifest")
			}
			if c.sev == Warning && ds.HasErrors() && !strings.Contains(c.name, "feature") {
				// A warning on its own should leave the manifest loadable
				for _, o := range ds {
					if o.Severity == Error {
						t.Errorf("a case that is only a warning also has %v", o)
					}
				}
			}
		})
	}
}

func TestTheCapsStopAManifestThatIsTooBig(t *testing.T) {
	tiny := func(path string) map[string]any {
		return map[string]any{"variants": map[string]any{"any": map[string]any{"path": path}}}
	}
	cases := []struct {
		name string
		edit func(r map[string]any)
		path string
	}{
		{"parts", func(r map[string]any) {
			parts := r["parts"].(map[string]any)
			for i := 0; i <= MaxParts; i++ {
				parts[fmt.Sprintf("p%d", i)] = tiny("a.png")
			}
		}, "parts"},
		{"variants", func(r map[string]any) {
			v := map[string]any{}
			for i := 0; i <= MaxVariants; i++ {
				v[fmt.Sprintf("k%d", i)] = map[string]any{"path": "a.png"}
			}
			r["parts"].(map[string]any)["border"].(map[string]any)["variants"] = v
		}, "parts.border.variants"},
		{"features", func(r map[string]any) {
			f := r["features"].(map[string]any)
			for i := 0; i <= MaxFeatures; i++ {
				f[fmt.Sprintf("f%d", i)] = map[string]any{"type": "bool"}
			}
		}, "features"},
		{"stack", func(r map[string]any) {
			var stack []any
			for i := 0; i <= MaxStack; i++ {
				stack = append(stack, map[string]any{"id": fmt.Sprintf("e%d", i), "part": "background"})
			}
			set(t, r, "layouts.normal.stack", stack)
		}, "layouts.normal.stack"},
		{"text boxes", func(r map[string]any) {
			boxes := r["layouts"].(map[string]any)["normal"].(map[string]any)["textBoxes"].(map[string]any)
			for i := 0; i <= MaxTextBoxes; i++ {
				boxes[fmt.Sprintf("b%d", i)] = map[string]any{"rect": map[string]any{"width": 1, "height": 1}, "text": map[string]any{"fontSize": 1}}
			}
		}, "layouts.normal.textBoxes"},
	}
	for _, c := range cases {
		_, ds := Decode(edit(t, c.edit))
		if d, ok := find(ds, c.path); !ok || d.Severity != Error || !strings.Contains(d.Message, "over the limit") {
			t.Errorf("%s: no over-the-limit error at %q in %d diagnostics", c.name, c.path, len(ds))
		}
	}
}

func TestPathProblemAcceptsOnlyPathsInsideTheBundle(t *testing.T) {
	good := []string{"a.png", "background/w.png", "a/b/c-d_e.PNG", "x.1.png"}
	for _, p := range good {
		if msg := pathProblem(p); msg != "" {
			t.Errorf("pathProblem(%q) = %q, want fine", p, msg)
		}
	}
	bad := []string{"", "/a.png", "../a.png", "a/./b.png", "a/../b.png", "a//b.png", "a\\b.png", "c:/a.png", "a.png/", "a\x00.png", "a.txt"}
	for _, p := range bad {
		if pathProblem(p) == "" {
			t.Errorf("pathProblem(%q) accepted it", p)
		}
	}
}

func TestEveryPairAndMaterialIsAKeyForAColorSlot(t *testing.T) {
	for _, key := range []string{"w", "u", "b", "r", "g", "wu", "wb", "wr", "wg", "ub", "ur", "ug", "br", "bg", "rg", "gold", "artifact", "colorless", "land", "vehicle", "any"} {
		if !colorKeys[key] {
			t.Errorf("%q is not accepted as a material", key)
		}
	}
	if len(colorKeys) != 21 {
		t.Errorf("%d material keys, want 21", len(colorKeys))
	}
}

func TestMergePatchFollowsRFC7386(t *testing.T) {
	cases := []struct{ doc, patch, want string }{
		{`{"a":"b"}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"b"}`, `{"b":"c"}`, `{"a":"b","b":"c"}`},
		{`{"a":"b"}`, `{"a":null}`, `{}`},
		{`{"a":"b","b":"c"}`, `{"a":null}`, `{"b":"c"}`},
		{`{"a":["b"]}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"c"}`, `{"a":["b"]}`, `{"a":["b"]}`},
		{`{"a":{"b":"c"}}`, `{"a":{"b":"d","c":null}}`, `{"a":{"b":"d"}}`},
		{`{"a":[{"b":"c"}]}`, `{"a":[1]}`, `{"a":[1]}`},
		{`["a","b"]`, `["c","d"]`, `["c","d"]`},
		{`{"a":"b"}`, `["c"]`, `["c"]`},
		{`{"a":"foo"}`, `null`, `null`},
		{`{"a":"foo"}`, `"bar"`, `"bar"`},
		{`{"e":null}`, `{"a":1}`, `{"a":1,"e":null}`},
		{`[1,2]`, `{"a":"b","c":null}`, `{"a":"b"}`},
		{`{}`, `{"a":{"bb":{"ccc":null}}}`, `{"a":{"bb":{}}}`},
	}
	for _, c := range cases {
		got, err := MergePatch([]byte(c.doc), []byte(c.patch))
		if err != nil || string(got) != c.want {
			t.Errorf("MergePatch(%s, %s) = %s, %v, want %s", c.doc, c.patch, got, err, c.want)
		}
	}
	if _, err := MergePatch([]byte(`{`), []byte(`{}`)); err == nil {
		t.Error("a broken document merged")
	}
	if _, err := MergePatch([]byte(`{}`), []byte(`{`)); err == nil {
		t.Error("a broken patch merged")
	}
}

func TestParseModeNamesEveryCompositingMode(t *testing.T) {
	for name := range modes {
		if _, err := ParseMode(name); err != nil {
			t.Errorf("ParseMode(%q): %v", name, err)
		}
		if _, err := ParseMode(strings.ToUpper(name)); err != nil {
			t.Errorf("ParseMode(%q) should ignore case: %v", strings.ToUpper(name), err)
		}
	}
	if m, err := ParseMode(""); err != nil || m != modes["normal"] {
		t.Errorf("an empty mode = %v, %v, want normal", m, err)
	}
	if _, err := ParseMode("burn"); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

func TestOverrideKeepsItsConditionAndPatchThroughJSON(t *testing.T) {
	var o Override
	if err := o.UnmarshalJSON([]byte(`{"condition":"land","rect":{"x":5}}`)); err != nil {
		t.Fatal(err)
	}
	if o.Condition != "land" || string(o.Patch) != `{"rect":{"x":5}}` {
		t.Errorf("override = %q %s", o.Condition, o.Patch)
	}
	out, err := o.MarshalJSON()
	if err != nil || string(out) != `{"condition":"land","rect":{"x":5}}` {
		t.Errorf("encoded = %s, %v", out, err)
	}
}
