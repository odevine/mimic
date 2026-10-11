package frame

import (
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// Kind says what a Material is made of
type Kind uint8

const (
	// KindNone is the value of a slot a card has nothing for, so only a layer's
	// color-invariant "any" image can draw
	KindNone Kind = iota
	// KindMono is one color, such as red
	KindMono
	// KindRun is two or more colors that run left to right across a frame, such
	// as the pair on a two-color card or the halves of a fuse bar
	KindRun
	KindGold
	KindArtifact
	KindColorless
	KindLand
	KindVehicle
	// KindNamed is a key that is not a color, such as the name of a transform icon
	// or the colors a color indicator shows
	KindNamed
)

// Material is what one slot of a frame is made of for a card. It names the image
// a layer draws for the slot and, for a run of colors, the order they run in. A
// Material is comparable and usable as a map key
type Material struct {
	kind Kind
	// letters is the lowercase WUBRG letters of a mono or run in the order they run
	// left to right, and the key of a named material
	letters string
}

// The materials that are not made of colors
var (
	Gold      = Material{kind: KindGold}
	Artifact  = Material{kind: KindArtifact}
	Colorless = Material{kind: KindColorless}
	Land      = Material{kind: KindLand}
	Vehicle   = Material{kind: KindVehicle}
)

// Mono is the material of one color
func Mono(c card.Color) Material {
	return Material{kind: KindMono, letters: letter(c)}
}

// Run is the material of colors that run left to right in the order given. One
// color is a Mono and none is no material
func Run(colors ...card.Color) Material {
	switch len(colors) {
	case 0:
		return Material{}
	case 1:
		return Mono(colors[0])
	}
	var b strings.Builder
	for _, c := range colors {
		b.WriteString(letter(c))
	}
	return Material{kind: KindRun, letters: b.String()}
}

// Dual is the material of two colors in the order a printed frame runs them. That
// is WUBRG order except for green and white, red and white, and green and blue,
// which the frames print the other way round. Any other count of colors is not a
// dual, and Dual reports no material for it
func Dual(colors []card.Color) Material {
	if len(colors) != 2 {
		return Material{}
	}
	ordered := canonical(colors)
	if flipped[letter(ordered[0])+letter(ordered[1])] {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	return Run(ordered...)
}

// flipped are the pairs, written in WUBRG order, whose frames run the second
// color first
var flipped = map[string]bool{"wg": true, "wr": true, "ug": true}

// Named is a material that is not a color, filed under key. An empty key is no
// material
func Named(key string) Material {
	if key == "" {
		return Material{}
	}
	return Material{kind: KindNamed, letters: key}
}

// ColorSet is the named material of a set of colors, filed under their letters in
// WUBRG order, which is how a color indicator's art is named. No colors is no
// material
func ColorSet(colors []card.Color) Material {
	var b strings.Builder
	for _, c := range canonical(colors) {
		b.WriteString(letter(c))
	}
	return Named(b.String())
}

// Kind reports what the material is made of
func (m Material) Kind() Kind { return m.kind }

// IsNone reports whether the slot has no material for the card
func (m Material) IsNone() bool { return m.kind == KindNone }

// Key is the name of the image that depicts the whole material, such as "w" or
// "gold", or "" for none. The colors of a run are in WUBRG order here, so a pair
// is filed as "wg" whichever way it runs
func (m Material) Key() string {
	switch m.kind {
	case KindMono, KindNamed:
		return m.letters
	case KindRun:
		return sortedLetters(m.letters)
	case KindGold:
		return "gold"
	case KindArtifact:
		return "artifact"
	case KindColorless:
		return "colorless"
	case KindLand:
		return "land"
	case KindVehicle:
		return "vehicle"
	}
	return ""
}

// Letters are the lowercase letters of the colors in the order they run left to
// right, one per color, or nil for a material that is not made of colors
func (m Material) Letters() []string {
	if m.kind != KindMono && m.kind != KindRun {
		return nil
	}
	out := make([]string, len(m.letters))
	for i := range m.letters {
		out[i] = m.letters[i : i+1]
	}
	return out
}

// String is the material as it reads in a message: the colors in the order they
// run, or the key of one that is not made of colors
func (m Material) String() string {
	if m.kind == KindMono || m.kind == KindRun {
		return m.letters
	}
	return m.Key()
}

// fromLetters is the material of colors given as lowercase letters in the order
// they run
func fromLetters(s string) Material {
	switch len(s) {
	case 0:
		return Material{}
	case 1:
		return Material{kind: KindMono, letters: s}
	}
	return Material{kind: KindRun, letters: s}
}

func letter(c card.Color) string { return strings.ToLower(string(c)) }

// sortedLetters puts a string of color letters in WUBRG order
func sortedLetters(s string) string {
	b := []byte(s)
	sort.SliceStable(b, func(i, j int) bool {
		return wubrgOrder[card.Color(strings.ToUpper(string(b[i])))] < wubrgOrder[card.Color(strings.ToUpper(string(b[j])))]
	})
	return string(b)
}
