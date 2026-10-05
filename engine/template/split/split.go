// Package split registers the modern frame for a split card, both halves in one
// portrait frame with each half printed on its side. A fuse card also draws its
// bar across the bottom of both halves. Its own Go is just this registration, the
// frame's look comes entirely from its manifest, rendered by the generic
// engine/render, which draws each half from its own face and turns the finished
// card upright
package split

import (
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

const (
	templateName = "split"
	description  = "The modern frame for split and fuse cards"
)

var supports = template.Supports{
	Roles: []template.Role{template.RoleSplit},
	Kinds: []template.Kind{template.KindStandard},
}

func init() {
	template.Register(templateName, description, supports, func() template.Template { return render.New(templateName) })
}
