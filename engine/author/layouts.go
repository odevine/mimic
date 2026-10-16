package author

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/odevine/mimic/engine/manifest"
)

// rawLayout is a layout's fields as written. Resolving works on the JSON so that a
// field the engine adds later passes through without a change here
type rawLayout map[string]json.RawMessage

// layoutFields are the parts of a layout that are inherited and patched
var inherited = []string{"canvas", "stack", "textBoxes", "art", "symbols", "sections"}

// authorFields are what a layout file may hold, beside the layout's own fields
var authorFields = []string{"extends", "patch"}

// layoutPatch is the edits a layout makes to what it extends
type layoutPatch struct {
	Stack *stackPatch `json:"stack,omitempty"`
	// TextBoxes, Symbols, Sections, and Canvas merge into the inherited ones as a
	// JSON merge patch, so an object merges and null removes. Art replaces the list
	TextBoxes json.RawMessage `json:"textBoxes,omitempty"`
	Symbols   json.RawMessage `json:"symbols,omitempty"`
	Sections  json.RawMessage `json:"sections,omitempty"`
	Canvas    json.RawMessage `json:"canvas,omitempty"`
	Art       json.RawMessage `json:"art,omitempty"`
}

// stackPatch edits an inherited stack. Entries are named by their id, which is the
// part's name when an entry has none. The edits apply in the order remove,
// replace, insert
type stackPatch struct {
	Remove  []string    `json:"remove,omitempty"`
	Replace []replaceOp `json:"replace,omitempty"`
	Insert  []insertOp  `json:"insert,omitempty"`
}

type replaceOp struct {
	ID   string            `json:"id"`
	With []json.RawMessage `json:"with"`
}

type insertOp struct {
	After   string            `json:"after,omitempty"`
	Before  string            `json:"before,omitempty"`
	Entries []json.RawMessage `json:"entries"`
}

// layoutResolver turns the layout files into complete layouts
type layoutResolver struct {
	files    map[string]rawLayout
	resolved map[string]rawLayout
	ds       manifest.Diagnostics
	visiting []string
}

func (r *layoutResolver) errorf(name, field, format string, args ...any) {
	path := "layouts." + name
	if field != "" {
		path += "." + field
	}
	r.ds = append(r.ds, manifest.Diagnostic{Severity: manifest.Error, Path: path, Message: fmt.Sprintf(format, args...)})
}

// resolveLayouts resolves every layout, reporting every problem it finds
func resolveLayouts(files map[string]rawLayout) (map[string]rawLayout, manifest.Diagnostics) {
	r := &layoutResolver{files: files, resolved: map[string]rawLayout{}}
	for _, name := range sortedKeys(files) {
		r.resolve(name)
	}
	if r.ds.HasErrors() {
		return nil, r.ds
	}
	return r.resolved, r.ds
}

func (r *layoutResolver) resolve(name string) (rawLayout, bool) {
	if l, ok := r.resolved[name]; ok {
		return l, l != nil
	}
	if slices.Contains(r.visiting, name) {
		r.errorf(name, "extends", "extends itself through %v", append(slices.Clone(r.visiting), name))
		return nil, false
	}
	r.visiting = append(r.visiting, name)
	defer func() { r.visiting = r.visiting[:len(r.visiting)-1] }()

	file := r.files[name]
	for field := range file {
		if field != "serves" && !slices.Contains(inherited, field) && !slices.Contains(authorFields, field) {
			r.errorf(name, field, "is not a field of a layout")
		}
	}
	out := rawLayout{}
	var extends []string
	if raw, ok := file["extends"]; ok {
		if err := json.Unmarshal(raw, &extends); err != nil {
			r.errorf(name, "extends", "is not a list of layout names")
		}
	}
	ok := true
	for i, parent := range extends {
		if _, exists := r.files[parent]; !exists {
			r.errorf(name, fmt.Sprintf("extends[%d]", i), "%q is not a layout of this template", parent)
			ok = false
			continue
		}
		p, pok := r.resolve(parent)
		if !pok {
			ok = false
			continue
		}
		if !r.overlay(name, out, p) {
			ok = false
		}
	}
	if !ok {
		r.resolved[name] = nil
		return nil, false
	}

	if serves, has := file["serves"]; has {
		out["serves"] = serves
	} else {
		r.errorf(name, "serves", "the layout does not say which shapes it serves")
	}
	for _, field := range inherited {
		raw, has := file[field]
		if !has {
			continue
		}
		// An inherited layout changes through its patch, except that a stack of its
		// own replaces the inherited one outright
		if len(extends) > 0 && field != "stack" {
			r.errorf(name, field, "cannot be set beside extends, so it belongs in the patch")
			continue
		}
		out[field] = raw
	}
	if raw, has := file["patch"]; has {
		if len(extends) == 0 {
			r.errorf(name, "patch", "has nothing to patch, because the layout extends none")
		} else {
			r.applyPatch(name, out, raw)
		}
	}
	for _, field := range []string{"stack", "textBoxes"} {
		if _, has := out[field]; !has {
			r.errorf(name, field, "the layout has none")
		}
	}
	r.resolved[name] = out
	return out, true
}

// overlay lays a parent over what the layouts before it gave. Stacks join in
// order, and an entry with an id already held replaces it in place. Text boxes,
// symbols, and sections join by name, and a canvas or a list of art windows
// replaces
func (r *layoutResolver) overlay(name string, into, parent rawLayout) bool {
	ok := true
	if raw, has := parent["stack"]; has {
		var base, add []json.RawMessage
		if prev, held := into["stack"]; held {
			_ = json.Unmarshal(prev, &base)
		}
		_ = json.Unmarshal(raw, &add)
		for _, e := range add {
			k, err := entryKey(e)
			if err != nil {
				r.errorf(name, "extends", "an inherited entry has no part: %v", err)
				ok = false
				continue
			}
			if i := indexOfEntry(base, k); i >= 0 {
				base[i] = e
			} else {
				base = append(base, e)
			}
		}
		into["stack"], _ = json.Marshal(base)
	}
	for _, field := range []string{"textBoxes", "symbols", "sections"} {
		raw, has := parent[field]
		if !has {
			continue
		}
		prev, held := into[field]
		if !held {
			prev = json.RawMessage(`{}`)
		}
		merged := map[string]json.RawMessage{}
		_ = json.Unmarshal(prev, &merged)
		var add map[string]json.RawMessage
		_ = json.Unmarshal(raw, &add)
		for k, v := range add {
			merged[k] = v
		}
		into[field], _ = json.Marshal(merged)
	}
	for _, field := range []string{"canvas", "art"} {
		if raw, has := parent[field]; has {
			into[field] = raw
		}
	}
	return ok
}

func (r *layoutResolver) applyPatch(name string, l rawLayout, raw json.RawMessage) {
	var p layoutPatch
	if err := decodeStrict(raw, &p); err != nil {
		r.errorf(name, "patch", "%v", err)
		return
	}
	if p.Stack != nil {
		r.patchStack(name, l, p.Stack)
	}
	for _, f := range []struct {
		field string
		patch json.RawMessage
		empty string
	}{{"textBoxes", p.TextBoxes, `{}`}, {"symbols", p.Symbols, `{}`}, {"sections", p.Sections, `{}`}, {"canvas", p.Canvas, `{}`}} {
		if f.patch == nil {
			continue
		}
		doc, has := l[f.field]
		if !has {
			doc = json.RawMessage(f.empty)
		}
		merged, err := manifest.MergePatch(doc, f.patch)
		if err != nil {
			r.errorf(name, "patch."+f.field, "%v", err)
			continue
		}
		l[f.field] = merged
	}
	if p.Art != nil {
		l["art"] = p.Art
	}
}

func (r *layoutResolver) patchStack(name string, l rawLayout, p *stackPatch) {
	var stack []json.RawMessage
	_ = json.Unmarshal(l["stack"], &stack)
	where := func(op string, i int, id string) int {
		at := indexOfEntry(stack, id)
		if at < 0 {
			r.errorf(name, fmt.Sprintf("patch.stack.%s[%d]", op, i), "the stack has no entry %q", id)
		}
		return at
	}
	for i, id := range p.Remove {
		if at := where("remove", i, id); at >= 0 {
			stack = slices.Delete(stack, at, at+1)
		}
	}
	for i, op := range p.Replace {
		if at := where("replace", i, op.ID); at >= 0 {
			stack = slices.Replace(stack, at, at+1, op.With...)
		}
	}
	for i, op := range p.Insert {
		switch {
		case (op.After == "") == (op.Before == ""):
			r.errorf(name, fmt.Sprintf("patch.stack.insert[%d]", i), "needs one of after and before")
		case op.After != "":
			if at := where("insert", i, op.After); at >= 0 {
				stack = slices.Insert(stack, at+1, op.Entries...)
			}
		default:
			if at := where("insert", i, op.Before); at >= 0 {
				stack = slices.Insert(stack, at, op.Entries...)
			}
		}
	}
	l["stack"], _ = json.Marshal(stack)
}

// entryKey is the name an entry is referred to by, its id or else its part
func entryKey(raw json.RawMessage) (string, error) {
	var e struct {
		ID   string `json:"id"`
		Part string `json:"part"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", err
	}
	if e.ID != "" {
		return e.ID, nil
	}
	if e.Part == "" {
		return "", fmt.Errorf("an entry has neither an id nor a part")
	}
	return e.Part, nil
}

func indexOfEntry(stack []json.RawMessage, key string) int {
	for i, e := range stack {
		if k, err := entryKey(e); err == nil && k == key {
			return i
		}
	}
	return -1
}
