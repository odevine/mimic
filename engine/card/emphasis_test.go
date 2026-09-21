package card

import "testing"

func TestEmphasisWords(t *testing.T) {
	if len(EmphasisWords) == 0 {
		t.Fatal("EmphasisWords is empty, the embedded catalog did not load")
	}
	// Ability words and flavor words emphasize; keyword abilities and actions do
	// not. Lookups are lowercase.
	italic := []string{"constellation", "channel", "landfall", "learn their secrets"}
	for _, w := range italic {
		if !EmphasisWords[w] {
			t.Errorf("%q should be an emphasis word", w)
		}
	}
	roman := []string{"suspend", "flying", "cycling", "flashback"}
	for _, w := range roman {
		if EmphasisWords[w] {
			t.Errorf("%q is a keyword and should not be an emphasis word", w)
		}
	}
}
