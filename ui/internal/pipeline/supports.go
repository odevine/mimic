package pipeline

import "github.com/odevine/mimic/engine/template"

// SupportsOf is the named template's registered supports, empty when this
// build has no template by that name
func SupportsOf(name string) template.Supports {
	for _, r := range template.List() {
		if r.Name == name {
			return r.Supports
		}
	}
	return template.Supports{}
}

// Supports is what the template renders currently go through can render
func (p *Pipeline) Supports() template.Supports {
	return SupportsOf(p.active.Load().Name)
}
