package frame

import "fmt"

// Slot names which of a card's frame materials a layer reads
type Slot uint8

const (
	// SlotNone is a layer with no color slot, which draws only its "any" image
	SlotNone Slot = iota
	SlotBackground
	SlotPinlines
	SlotTwins
	SlotPTBox
	SlotCrown
	SlotIndicator
	SlotTransformIcon
	SlotFuse
	SlotFusePinlines
	SlotPrototype
)

var slotNames = [...]string{
	SlotNone:          "",
	SlotBackground:    "background",
	SlotPinlines:      "pinlines",
	SlotTwins:         "twins",
	SlotPTBox:         "ptBox",
	SlotCrown:         "crown",
	SlotIndicator:     "indicator",
	SlotTransformIcon: "transform_icon",
	SlotFuse:          "fuse",
	SlotFusePinlines:  "fuse_pinlines",
	SlotPrototype:     "prototype",
}

// String is the name a manifest gives the slot
func (s Slot) String() string {
	if int(s) < len(slotNames) {
		return slotNames[s]
	}
	return fmt.Sprintf("Slot(%d)", s)
}

// SlotNames lists the names a manifest gives its slots, in the engine's order,
// without the empty name of a part that has none
func SlotNames() []string { return append([]string(nil), slotNames[1:]...) }

// ParseSlot reads the name a manifest gives a slot. The empty name is SlotNone,
// and a name the engine does not have is an error
func ParseSlot(name string) (Slot, error) {
	for s, n := range slotNames {
		if n == name {
			return Slot(s), nil
		}
	}
	return SlotNone, fmt.Errorf("unknown color slot %q", name)
}

// Material returns what the slot is made of for the card these keys were read
// from
func (k Keys) Material(s Slot) Material {
	switch s {
	case SlotBackground:
		return k.Background
	case SlotPinlines:
		return k.Pinlines
	case SlotTwins:
		return k.Twins
	case SlotPTBox:
		return k.PTBox
	case SlotCrown:
		return k.Crown
	case SlotIndicator:
		return k.Indicator
	case SlotTransformIcon:
		return k.TransformIcon
	case SlotFuse:
		return k.FuseKey
	case SlotFusePinlines:
		return k.FusePinKey
	case SlotPrototype:
		return k.Prototype
	}
	return Material{}
}

// MaterialNamed is Material for a slot given by the name a manifest uses. A name
// the engine does not have, and the empty name, give no material
func (k Keys) MaterialNamed(name string) Material {
	s, err := ParseSlot(name)
	if err != nil {
		return Material{}
	}
	return k.Material(s)
}
