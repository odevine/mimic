// Package transform registers the modern frame for both faces of a transform
// double-faced card: the normal frame with the transform icon in its corner,
// and on the back a darker frame with light text. Its own Go is just this
// registration, the frame's look comes entirely from its manifest, rendered by
// the generic engine/render, which picks each face's layers by the front and
// back conditions
package transform

import (
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

const (
	templateName = "transform"
	description  = "The modern frame for both faces of transform cards"
)

var supports = template.Supports{
	Roles: []template.Role{template.RoleTransformFront, template.RoleTransformBack},
	Kinds: []template.Kind{template.KindStandard},
}

func init() {
	template.Register(templateName, description, supports, func() template.Template { return render.New(templateName) })
}
