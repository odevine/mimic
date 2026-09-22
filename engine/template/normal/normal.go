// Package normal registers the standard modern Magic card frame: a
// WUBRG-colored border, pinlines, and legendary crown around a rectangular
// art window. Its own Go is just this registration; the frame's actual look
// comes entirely from its manifest, rendered by the generic engine/render
package normal

import (
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

const (
	templateName = "normal"
	description  = "The standard, modern Magic card frame"
)

func init() {
	template.Register(templateName, description, func() template.Template { return render.New(templateName) })
}
