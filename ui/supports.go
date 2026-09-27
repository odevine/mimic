package main

import "github.com/odevine/mimic/engine/template"

// supportsOf is the named template's registered supports, empty when this
// build has no template by that name
func supportsOf(name string) template.Supports {
	for _, r := range template.List() {
		if r.Name == name {
			return r.Supports
		}
	}
	return template.Supports{}
}

// supports is what the template renders currently go through can render
func (p *renderPipeline) supports() template.Supports {
	return supportsOf(p.active.Load().name)
}
