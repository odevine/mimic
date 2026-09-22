// Package all blank-imports every template this engine ships, so registering
// a new one under engine/template/<name> is the one change needed for it to
// show up in template.List() and be covered by this package's registry test.
// Whatever assembles a binary that needs every template available (rather
// than a caller-chosen subset) imports this package instead of each template
// package individually
package all

import (
	_ "github.com/odevine/mimic/engine/template/normal"
)
