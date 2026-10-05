package template

import (
	"fmt"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
)

// Role is where a face sits on the physical card. A template frame is drawn for
// one role, since a transform front carries a flip icon a single card does not,
// and a split card lays two halves into one frame
type Role string

const (
	RoleSingle         Role = "single"
	RoleSplit          Role = "split"
	RoleAftermath      Role = "aftermath"
	RoleAdventure      Role = "adventure"
	RoleTransformFront Role = "transform_front"
	RoleTransformBack  Role = "transform_back"
	RoleMDFCFront      Role = "mdfc_front"
	RoleMDFCBack       Role = "mdfc_back"
	// RoleUnknown is a layout the engine does not classify yet, such as flip or
	// meld. No template may declare it, so no template renders one
	RoleUnknown Role = "unknown"
)

// Kind is the frame a face needs, independent of where it sits on the card
type Kind string

const (
	KindStandard     Kind = "standard"
	KindBasicLand    Kind = "basic_land"
	KindPlaneswalker Kind = "planeswalker"
	KindSaga         Kind = "saga"
	KindClass        Kind = "class"
	KindCase         Kind = "case"
	KindRoom         Kind = "room"
	KindLeveler      Kind = "leveler"
	KindMutate       Kind = "mutate"
	KindPrototype    Kind = "prototype"
	KindBattle       Kind = "battle"
	KindToken        Kind = "token"
	KindPlane        Kind = "plane"
	// KindUnknown is a card the engine does not classify yet, such as a scheme
	// or an emblem. No template may declare it, so no template renders one
	KindUnknown Kind = "unknown"
)

var (
	knownRoles = map[Role]bool{
		RoleSingle: true, RoleSplit: true, RoleAftermath: true, RoleAdventure: true,
		RoleTransformFront: true, RoleTransformBack: true,
		RoleMDFCFront: true, RoleMDFCBack: true,
	}
	knownKinds = map[Kind]bool{
		KindStandard: true, KindBasicLand: true, KindPlaneswalker: true,
		KindSaga: true, KindClass: true, KindCase: true, KindRoom: true, KindLeveler: true,
		KindMutate: true, KindPrototype: true, KindBattle: true,
		KindToken: true, KindPlane: true,
	}
)

// Shape is one renderable face's role and kind, the pair a template's Supports
// is checked against
type Shape struct {
	Role Role `json:"role"`
	Kind Kind `json:"kind"`
}

func (s Shape) String() string { return string(s.Kind) + " (" + string(s.Role) + ")" }

// Supports is what a template can render. A face is supported when its role is
// in Roles and its kind is in Kinds, so a template covering several roles or
// kinds covers every pairing of them
type Supports struct {
	Roles []Role `json:"roles"`
	Kinds []Kind `json:"kinds"`
}

// Allows reports whether a face of shape s can render with this template
func (s Supports) Allows(sh Shape) bool {
	roleOK, kindOK := false, false
	for _, r := range s.Roles {
		roleOK = roleOK || r == sh.Role
	}
	for _, k := range s.Kinds {
		kindOK = kindOK || k == sh.Kind
	}
	return roleOK && kindOK
}

// validate rejects an empty list and any role or kind outside the engine's
// vocabulary, including the unknown ones, which no template may claim
func (s Supports) validate() error {
	if len(s.Roles) == 0 || len(s.Kinds) == 0 {
		return fmt.Errorf("supports needs at least one role and one kind")
	}
	for _, r := range s.Roles {
		if !knownRoles[r] {
			return fmt.Errorf("unknown role %q", r)
		}
	}
	for _, k := range s.Kinds {
		if !knownKinds[k] {
			return fmt.Errorf("unknown kind %q", k)
		}
	}
	return nil
}

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
		return []Shape{{RoleSingle, kind}}
	case "split":
		return []Shape{splitShape(d)}
	case "adventure":
		return []Shape{{RoleAdventure, KindStandard}}
	case "transform", "battle", "double_faced_token":
		return doubleFaced(d, RoleTransformFront, RoleTransformBack)
	case "modal_dfc":
		return doubleFaced(d, RoleMDFCFront, RoleMDFCBack)
	}
	return []Shape{{RoleUnknown, KindUnknown}}
}

// doubleFaced classifies each face of a two-faced card by its own type line,
// so a planeswalker back or a saga front gets its own frame
func doubleFaced(d *card.Data, front, back Role) []Shape {
	if len(d.Faces) != 2 {
		return []Shape{{RoleUnknown, KindUnknown}}
	}
	return []Shape{
		{front, typeLineKind(d.Faces[0].TypeLine)},
		{back, typeLineKind(d.Faces[1].TypeLine)},
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

// UnsupportedError is what a registered template's Render returns for a face
// its Supports does not allow, so a batch can tell a card no template here
// renders from one that failed partway through
type UnsupportedError struct {
	Template string
	Shape    Shape
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("template %q does not render %s faces", e.Template, e.Shape)
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

// Side reports which face of a double-faced card a shape is, from its role. A
// face on one side of the card, such as a single, split, or adventure card, is
// frame.Single
func (s Shape) Side() frame.Side {
	switch {
	case strings.HasSuffix(string(s.Role), "_front"):
		return frame.Front
	case strings.HasSuffix(string(s.Role), "_back"):
		return frame.Back
	}
	return frame.Single
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
			return Shape{RoleAftermath, KindStandard}
		}
	}
	if len(d.Faces) > 1 && strings.HasPrefix(d.Faces[1].OracleText, "Aftermath (") {
		return Shape{RoleAftermath, KindStandard}
	}
	return Shape{RoleSplit, typeLineKind(d.TypeLine)}
}
