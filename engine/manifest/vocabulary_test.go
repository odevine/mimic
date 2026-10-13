package manifest

import (
	"slices"
	"testing"

	"github.com/odevine/mimic/engine/frame"
)

func TestVariantKeysAreAllValidForTheirSlot(t *testing.T) {
	for _, name := range append([]string{""}, frame.SlotNames()...) {
		slot, err := frame.ParseSlot(name)
		if err != nil {
			t.Fatal(err)
		}
		keys := VariantKeys(slot)
		if len(keys) == 0 || keys[0] != "any" {
			t.Errorf("slot %q keys %v, want any first", name, keys)
		}
		for _, k := range keys {
			if problem, _ := variantKeyProblem(slot, k); problem != "" {
				t.Errorf("slot %q lists %q, which the validator refuses: %s", name, k, problem)
			}
		}
	}
}

func TestVariantKeysOfAColorSlotAreEveryMaterial(t *testing.T) {
	keys := VariantKeys(frame.SlotPinlines)
	if len(keys) != 21 {
		t.Fatalf("%d keys: %v", len(keys), keys)
	}
	for _, want := range []string{"w", "wg", "gold", "land"} {
		if !slices.Contains(keys, want) {
			t.Errorf("no %q in %v", want, keys)
		}
	}
	if got := VariantKeys(frame.SlotNone); len(got) != 1 {
		t.Errorf("a part with no slot lists %v", got)
	}
	if n := len(VariantKeys(frame.SlotIndicator)); n != 32 {
		t.Errorf("indicator lists %d keys, want any and the 31 color sets", n)
	}
}
