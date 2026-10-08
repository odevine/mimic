package desktop

import (
	"context"
	"encoding"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/jobs"
)

// The page and the services meet at JSON, and a renamed field or a changed type
// only fails when someone uses the screen that reads it. This test writes down
// the signature of every bound method and the JSON shape of every type those
// signatures use, and holds the result against testdata/bridge.golden. A change
// to the bridge shows up as a diff to review, and -update rewrites the file

var update = flag.Bool("update", false, "rewrite the bridge golden file")

const bridgeGolden = "testdata/bridge.golden"

// bridgeTypes are the bound types the golden covers: the ones api.js reaches
// through boundTypes, and the job service it calls by its own name
func bridgeTypes() map[string]reflect.Type {
	all := map[string]reflect.Type{"Jobs": reflect.TypeFor[*jobs.Service]()}
	for name, typ := range boundTypes {
		all[name] = typ
	}
	return all
}

var (
	ctxType       = reflect.TypeFor[context.Context]()
	errType       = reflect.TypeFor[error]()
	timeType      = reflect.TypeFor[time.Time]()
	durationType  = reflect.TypeFor[time.Duration]()
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// A type with its own MarshalJSON sends something other than its fields, so the
// walk cannot read its shape. Each one is described by a stand-in struct with
// the fields it sends, and a test holds the stand-in to what the type marshals
var standIns = map[reflect.Type]reflect.Type{
	reflect.TypeFor[cardlist.Resolved](): reflect.TypeFor[resolvedWire](),
}

// resolvedWire is what cardlist.Resolved marshals to: its own fields, with the
// cards carrying their shapes
type resolvedWire struct {
	Index      int                    `json:"index"`
	Status     string                 `json:"status"`
	Card       *cardlist.ShapedCard   `json:"card,omitempty"`
	Candidates []*cardlist.ShapedCard `json:"candidates,omitempty"`
	Cards      []*cardlist.ShapedCard `json:"cards,omitempty"`
	Note       string                 `json:"note,omitempty"`
}

// shape renders the JSON shape of types, one block per named type
type shape struct {
	named map[string]string
	todo  []reflect.Type
}

// ref names a type as a signature or field would mention it, queueing the named
// types that need their own block
func (s *shape) ref(t reflect.Type) string {
	base := t
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if _, ok := standIns[base]; ok {
		s.queue(base)
		return base.String()
	}
	switch {
	case t == timeType:
		return "time"
	case t == durationType:
		return "duration"
	case t.Implements(jsonMarshaler) || t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(textMarshaler):
		return "custom " + t.String()
	}
	switch t.Kind() {
	case reflect.Pointer:
		return s.ref(t.Elem())
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes"
		}
		return "[]" + s.ref(t.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), s.ref(t.Elem()))
	case reflect.Map:
		return "map[" + s.ref(t.Key()) + "]" + s.ref(t.Elem())
	case reflect.Interface:
		return "any"
	case reflect.Struct, reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		if t.Name() == "" {
			return s.inline(t)
		}
		s.queue(t)
		return t.String()
	}
	return "unsupported " + t.String()
}

func (s *shape) queue(t reflect.Type) {
	name := t.String()
	if _, seen := s.named[name]; seen {
		return
	}
	s.named[name] = "" // reserved, so a type that refers to itself terminates
	s.todo = append(s.todo, t)
}

// inline renders an unnamed struct in place
func (s *shape) inline(t reflect.Type) string {
	if t.Kind() != reflect.Struct {
		return t.Kind().String()
	}
	return "struct{" + strings.Join(s.fields(t), "; ") + "}"
}

// fields lists a struct's JSON fields the way encoding/json sees them: renamed
// by tag, unexported and "-" fields left out, untagged embedded structs flattened
func (s *shape) fields(t reflect.Type) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			et := f.Type
			if et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				out = append(out, s.fields(et)...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		line := name + " " + s.ref(f.Type)
		if strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero") {
			line += " (omitempty)"
		}
		if strings.Contains(opts, "string") {
			line += " (string)"
		}
		out = append(out, line)
	}
	return out
}

// block renders one named type
func (s *shape) block(t reflect.Type) string {
	if stand, ok := standIns[t]; ok {
		var b strings.Builder
		fmt.Fprintf(&b, "%s struct {\n", t)
		for _, f := range s.fields(stand) {
			fmt.Fprintf(&b, "\t%s\n", f)
		}
		b.WriteString("}")
		return b.String()
	}
	if t.Kind() != reflect.Struct {
		return fmt.Sprintf("%s = %s", t, t.Kind())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s struct {\n", t)
	for _, f := range s.fields(t) {
		fmt.Fprintf(&b, "\t%s\n", f)
	}
	b.WriteString("}")
	return b.String()
}

// bridgeSurface renders every bound method's signature, then the types they use
func bridgeSurface() string {
	s := &shape{named: map[string]string{}}
	types := bridgeTypes()
	services := make([]string, 0, len(types))
	for name := range types {
		services = append(services, name)
	}
	sort.Strings(services)

	var methods []string
	for _, service := range services {
		typ := types[service]
		for i := range typ.NumMethod() {
			m := typ.Method(i)
			if slices.Contains(hooks, m.Name) {
				continue
			}
			var in, out []string
			for j := 1; j < m.Type.NumIn(); j++ {
				if p := m.Type.In(j); p != ctxType {
					in = append(in, s.ref(p))
				}
			}
			throws := false
			for j := range m.Type.NumOut() {
				if r := m.Type.Out(j); r == errType {
					throws = true
				} else {
					out = append(out, s.ref(r))
				}
			}
			line := fmt.Sprintf("%s.%s(%s)", service, m.Name, strings.Join(in, ", "))
			if len(out) > 0 {
				line += " -> " + strings.Join(out, ", ")
			}
			if throws {
				line += " !"
			}
			methods = append(methods, line)
		}
	}

	for len(s.todo) > 0 {
		t := s.todo[0]
		s.todo = s.todo[1:]
		s.named[t.String()] = s.block(t)
	}
	names := make([]string, 0, len(s.named))
	for name := range s.named {
		names = append(names, name)
	}
	sort.Strings(names)
	blocks := make([]string, len(names))
	for i, name := range names {
		blocks[i] = s.named[name]
	}

	return "# Every bound method, as Service.Method(arguments) -> results. A trailing ! means it can fail,\n" +
		"# which the page sees as a rejected call\n\n" +
		strings.Join(methods, "\n") + "\n\n# The JSON shape of every type those signatures use\n\n" +
		strings.Join(blocks, "\n\n") + "\n"
}

func TestBridgeMatchesGolden(t *testing.T) {
	got := bridgeSurface()
	if *update {
		if err := os.WriteFile(bridgeGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(bridgeGolden)
	if err != nil {
		t.Fatalf("%v: run go test ./internal/desktop -run Bridge -update to write it", err)
	}
	if got != string(want) {
		t.Errorf("the bridge changed. If that is intended, run go test ./internal/desktop -run Bridge -update and review the diff.\n%s", firstDifference(string(want), got))
	}
}

// firstDifference points at where two texts part, with a little context
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\n  golden: %q\n  now:    %q", i+1, wl, gl)
		}
	}
	return ""
}

func TestBridgeCoversEveryBoundType(t *testing.T) {
	covered := bridgeTypes()
	for name := range boundTypes {
		if _, ok := covered[name]; !ok {
			t.Errorf("%s is bound but the bridge golden skips it", name)
		}
	}
	if surface := bridgeSurface(); !strings.Contains(surface, "Render.Start(") || !strings.Contains(surface, "render.Request struct") {
		t.Errorf("the surface lost the render call, so the walk has broken:\n%s", surface)
	}
}

// jsonKeys marshals v and lists the top-level keys it produced
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestStandInsMatchWhatTheTypesMarshal(t *testing.T) {
	d := &card.Data{Name: "Sol Ring"}
	got := jsonKeys(t, cardlist.Resolved{Index: 1, Status: "ok", Card: d, Candidates: []*card.Data{d}, Cards: []*card.Data{d}, Note: "n"})
	wire := resolvedWire{
		Index: 1, Status: "ok", Note: "n",
		Card:       cardlist.Shaped(d),
		Candidates: cardlist.ShapedAll([]*card.Data{d}),
		Cards:      cardlist.ShapedAll([]*card.Data{d}),
	}
	if want := jsonKeys(t, wire); !slices.Equal(got, want) {
		t.Errorf("cardlist.Resolved marshals the keys %v and its stand-in %v, so update resolvedWire", got, want)
	}
	// The cards inside must carry their shapes, which is the reason for the override
	raw, _ := json.Marshal(cardlist.Resolved{Card: d})
	if !strings.Contains(string(raw), `"shapes"`) {
		t.Errorf("a resolved card is sent without its shapes: %s", raw)
	}
}
