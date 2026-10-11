package template

import (
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
)

// Role, Kind, and Shape are the engine's vocabulary for where a face sits and
// what frame it needs, defined beside the rest of a card's frame rules
type (
	Role  = frame.Role
	Kind  = frame.Kind
	Shape = frame.Shape
)

const (
	RoleSingle         = frame.RoleSingle
	RoleSplit          = frame.RoleSplit
	RoleAftermath      = frame.RoleAftermath
	RoleAdventure      = frame.RoleAdventure
	RoleTransformFront = frame.RoleTransformFront
	RoleTransformBack  = frame.RoleTransformBack
	RoleMDFCFront      = frame.RoleMDFCFront
	RoleMDFCBack       = frame.RoleMDFCBack
	RoleUnknown        = frame.RoleUnknown

	KindStandard     = frame.KindStandard
	KindBasicLand    = frame.KindBasicLand
	KindPlaneswalker = frame.KindPlaneswalker
	KindSaga         = frame.KindSaga
	KindClass        = frame.KindClass
	KindCase         = frame.KindCase
	KindRoom         = frame.KindRoom
	KindLeveler      = frame.KindLeveler
	KindMutate       = frame.KindMutate
	KindPrototype    = frame.KindPrototype
	KindBattle       = frame.KindBattle
	KindToken        = frame.KindToken
	KindPlane        = frame.KindPlane
	KindUnknown      = frame.KindUnknown
)

// layoutKinds are the Scryfall layouts that name a single-faced card's kind
// outright, where the type line alone would not
var layoutKinds = map[string]Kind{
	"leveler":   KindLeveler,
	"mutate":    KindMutate,
	"prototype": KindPrototype,
	"saga":      KindSaga,
	"class":     KindClass,
	"case":      KindCase,
	"token":     KindToken,
	"planar":    KindPlane,
}

// Classify reports the shape of each image a card renders to, in order. A
// double-faced card renders two, front then back, which RenderRequest.Face
// indexes. A split or adventure card lays both halves into one frame, so it
// renders one. A card with no layout, such as one typed in by hand, is a
// single face classified from its type line
func Classify(d *card.Data) []Shape {
	switch d.Layout {
	case "", "normal", "leveler", "mutate", "prototype", "saga", "class", "case", "token", "planar":
		kind, ok := layoutKinds[d.Layout]
		if !ok {
			kind = typeLineKind(d.TypeLine)
		}
		return []Shape{{Role: RoleSingle, Kind: kind}}
	case "split":
		return []Shape{splitShape(d)}
	case "adventure":
		return []Shape{{Role: RoleAdventure, Kind: KindStandard}}
	case "transform", "battle", "double_faced_token":
		return doubleFaced(d, RoleTransformFront, RoleTransformBack)
	case "modal_dfc":
		return doubleFaced(d, RoleMDFCFront, RoleMDFCBack)
	}
	return []Shape{{Role: RoleUnknown, Kind: KindUnknown}}
}

// doubleFaced classifies each face of a two-faced card by its own type line,
// so a planeswalker back or a saga front gets its own frame
func doubleFaced(d *card.Data, front, back Role) []Shape {
	if len(d.Faces) != 2 {
		return []Shape{{Role: RoleUnknown, Kind: KindUnknown}}
	}
	return []Shape{
		{Role: front, Kind: typeLineKind(d.Faces[0].TypeLine)},
		{Role: back, Kind: typeLineKind(d.Faces[1].TypeLine)},
	}
}

// typeLineKind reads a face's kind from its type line. The checks run in
// order, so a token planeswalker is a token and a basic snow land is a basic
// land
func typeLineKind(typeLine string) Kind {
	types, subtypes, _ := strings.Cut(strings.ToLower(typeLine), "—")
	has := func(field, word string) bool {
		for _, w := range strings.Fields(field) {
			if w == word {
				return true
			}
		}
		return false
	}
	switch {
	case has(types, "token"):
		return KindToken
	case has(types, "planeswalker"):
		return KindPlaneswalker
	case has(types, "battle"):
		return KindBattle
	case has(types, "plane"):
		return KindPlane
	case has(types, "enchantment") && has(subtypes, "saga"):
		return KindSaga
	case has(types, "enchantment") && has(subtypes, "class"):
		return KindClass
	case has(types, "enchantment") && has(subtypes, "case"):
		return KindCase
	case has(types, "enchantment") && has(subtypes, "room"):
		return KindRoom
	case has(types, "basic") && has(types, "land"):
		return KindBasicLand
	}
	return KindStandard
}

// FaceCard is the card data the requested face prints. A double-faced card's
// back face carries its own name, text, and art, while a single, split, or
// adventure card renders as the card itself
func (r RenderRequest) FaceCard() *card.Data {
	shapes := Classify(r.Card)
	if r.Face <= 0 || r.Face >= len(shapes) {
		return r.Card
	}
	return r.Card.Face(r.Face)
}

// FaceSide is the side of the card the requested face prints on, which a
// double-faced frame reads through frame.DeriveFace
func (r RenderRequest) FaceSide() frame.Side {
	shapes := Classify(r.Card)
	if r.Face < 0 || r.Face >= len(shapes) {
		return frame.Single
	}
	return shapes[r.Face].Side()
}

// splitShape tells apart the frames Scryfall files under its split layout. An
// Aftermath card prints its second half turned the other way, and a Room is an
// enchantment frame with a door on each half, so neither is a classic split or
// fuse card
func splitShape(d *card.Data) Shape {
	for _, k := range d.Keywords {
		if strings.EqualFold(k, "aftermath") {
			return Shape{Role: RoleAftermath, Kind: KindStandard}
		}
	}
	if len(d.Faces) > 1 && strings.HasPrefix(d.Faces[1].OracleText, "Aftermath (") {
		return Shape{Role: RoleAftermath, Kind: KindStandard}
	}
	return Shape{Role: RoleSplit, Kind: typeLineKind(d.TypeLine)}
}
