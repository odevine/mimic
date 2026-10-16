package frame

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func TestParseSlotReadsEveryName(t *testing.T) {
	names := map[string]Slot{
		"":               SlotNone,
		"background":     SlotBackground,
		"pinlines":       SlotPinlines,
		"twins":          SlotTwins,
		"ptBox":          SlotPTBox,
		"crown":          SlotCrown,
		"indicator":      SlotIndicator,
		"transform_icon": SlotTransformIcon,
		"fuse":           SlotFuse,
		"fuse_pinlines":  SlotFusePinlines,
	}
	for name, want := range names {
		got, err := ParseSlot(name)
		if err != nil || got != want {
			t.Errorf("ParseSlot(%q) = %v, %v, want %v", name, got, err, want)
		}
		if got.String() != name {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), name)
		}
	}
}

func TestParseSlotRejectsAnUnknownName(t *testing.T) {
	for _, name := range []string{"border", "Background", "pt_box", " crown"} {
		if _, err := ParseSlot(name); err == nil {
			t.Errorf("ParseSlot(%q) succeeded, want an error", name)
		}
	}
}

func TestMaterialReadsEverySlot(t *testing.T) {
	k := Keys{
		Background:    Land,
		Pinlines:      Mono(card.Red),
		Twins:         Gold,
		PTBox:         Vehicle,
		Crown:         Artifact,
		Indicator:     Named("ub"),
		TransformIcon: Named("sunmoondfc"),
		FuseKey:       Run(card.Red, card.White),
		FusePinKey:    Colorless,
		Prototype:     Mono(card.Blue),
	}
	cases := map[Slot]Material{
		SlotNone:          {},
		SlotBackground:    Land,
		SlotPinlines:      Mono(card.Red),
		SlotTwins:         Gold,
		SlotPTBox:         Vehicle,
		SlotCrown:         Artifact,
		SlotIndicator:     Named("ub"),
		SlotTransformIcon: Named("sunmoondfc"),
		SlotFuse:          Run(card.Red, card.White),
		SlotFusePinlines:  Colorless,
		SlotPrototype:     Mono(card.Blue),
	}
	for slot, want := range cases {
		if got := k.Material(slot); got != want {
			t.Errorf("Material(%q) = %q, want %q", slot, got, want)
		}
	}
}

func TestSlotNamesParseBack(t *testing.T) {
	names := SlotNames()
	if len(names) != int(SlotPrototype) {
		t.Fatalf("%d names: %v", len(names), names)
	}
	for _, n := range names {
		if s, err := ParseSlot(n); err != nil || s.String() != n || s == SlotNone {
			t.Errorf("%q parses to %v, %v", n, s, err)
		}
	}
}
