package main

import (
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// parseColors reads WUBRG(C) letters into a color slice, ignoring anything else
// and dropping duplicates so the frame logic sees a clean set
func parseColors(s string) []card.Color {
	seen := make(map[card.Color]bool)
	var out []card.Color
	for _, r := range strings.ToUpper(s) {
		c := card.Color(string(r))
		switch c {
		case card.White, card.Blue, card.Black, card.Red, card.Green, card.Colorless:
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
