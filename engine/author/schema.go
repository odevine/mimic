package author

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// SourceSchema is the JSON Schema of template.json, written from the Go types that
// the packer reads, so an editor checks the file against the fields the packer
// accepts
func SourceSchema() ([]byte, error) {
	g := newSchemaGen()
	root := g.object(reflect.TypeOf(Source{}))
	root["$schema"] = schemaDialect
	root["title"] = "mimic template.json"
	root["required"] = []string{"contract", "id", "version", "name", "parts"}
	root["$defs"] = g.defs
	return marshalSchema(root)
}

// LayoutSchema is the JSON Schema of a file in a template's layouts folder
func LayoutSchema() ([]byte, error) {
	g := newSchemaGen()
	root := g.object(reflect.TypeOf(layoutFile{}))
	root["$schema"] = schemaDialect
	root["title"] = "mimic layout"
	root["required"] = []string{"serves"}
	root["$defs"] = g.defs
	return marshalSchema(root)
}

const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// layoutFile is what a layout file may hold, for the schema
type layoutFile struct {
	manifest.Layout
	Extends []string        `json:"extends,omitempty"`
	Patch   *layoutPatchDoc `json:"patch,omitempty"`
}

// layoutPatchDoc is a layout's patch, for the schema
type layoutPatchDoc struct {
	Stack     *stackPatch    `json:"stack,omitempty"`
	TextBoxes map[string]any `json:"textBoxes,omitempty"`
	Symbols   map[string]any `json:"symbols,omitempty"`
	Canvas    map[string]any `json:"canvas,omitempty"`
	Art       []any          `json:"art,omitempty"`
}

func marshalSchema(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

type schemaGen struct {
	defs map[string]any
}

func newSchemaGen() *schemaGen { return &schemaGen{defs: map[string]any{}} }

var (
	rawMessageType = reflect.TypeOf(json.RawMessage(nil))
	overrideType   = reflect.TypeOf(manifest.Override{})
	shapeType      = reflect.TypeOf(frame.Shape{})
)

// of is the schema of a Go type
func (g *schemaGen) of(t reflect.Type) any {
	switch {
	case t == rawMessageType:
		return map[string]any{}
	case t == overrideType:
		// A condition and the changes it makes to a text box, which are fields of a text box
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{"condition": map[string]any{"type": "string"}},
			"required":   []string{"condition"},
		}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.of(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": g.of(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.of(t.Elem())}
	case reflect.Struct:
		name := t.Name()
		if _, ok := g.defs[name]; !ok {
			g.defs[name] = map[string]any{} // a placeholder, since a type may contain itself
			g.defs[name] = g.object(t)
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	panic(fmt.Sprintf("schema: no schema for %s", t))
}

// object is the schema of a struct as an object that allows only its fields
func (g *schemaGen) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	g.fields(t, props)
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if t == shapeType {
		props["role"] = map[string]any{"type": "string", "enum": frame.Roles()}
		props["kind"] = map[string]any{"type": "string", "enum": frame.Kinds()}
		out["required"] = []string{"role", "kind"}
	}
	return out
}

// fields adds the JSON fields of a struct, with an embedded struct's fields as its own
func (g *schemaGen) fields(t reflect.Type, props map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || !f.IsExported() {
			continue
		}
		if f.Anonymous && tag == "" {
			g.fields(f.Type, props)
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		props[name] = g.of(f.Type)
	}
}
