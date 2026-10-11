package frame

import "strings"

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

// Side reports which face of a double-faced card a shape is, from its role. A
// face on one side of the card, such as a single, split, or adventure card, is
// Single
func (s Shape) Side() Side {
	switch {
	case strings.HasSuffix(string(s.Role), "_front"):
		return Front
	case strings.HasSuffix(string(s.Role), "_back"):
		return Back
	}
	return Single
}

// KnownRole reports whether a template may declare the role, which excludes the
// unknown role the engine gives a layout it does not classify
func KnownRole(r Role) bool { return knownRoles[r] }

// KnownKind reports whether a template may declare the kind, which excludes the
// unknown kind the engine gives a card it does not classify
func KnownKind(k Kind) bool { return knownKinds[k] }
