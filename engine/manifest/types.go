// Package manifest defines the resolved form of a template, the one a bundle
// carries and the engine reads, and decodes and checks it. The resolved form
// holds no inheritance and names no directory, so everything a render needs is
// written out: each part lists its images, and each layout lists its complete
// stack, text boxes, and art windows. Decoding returns diagnostics with a path to
// each problem, so the loader, the validate command, and an editor share one
// implementation
package manifest

import (
	"encoding/json"

	"github.com/odevine/mimic/engine/frame"
)

// Contract is the version of the resolved form this package reads, as MAJOR.MINOR.
// A manifest with a different major is refused, and one with a newer minor loads
// with a warning because a minor release only adds optional fields. Minor 1 added
// the fonts and roles tables
const Contract = "1.1"

// Manifest is a template as the engine reads it
type Manifest struct {
	Contract    string `json:"contract"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Author      Author `json:"author,omitempty"`
	License     string `json:"license,omitempty"`
	// Credits names the people whose art or frames the template was cut from, so
	// the credit travels with the art
	Credits  []Credit           `json:"credits,omitempty"`
	Parts    map[string]Part    `json:"parts"`
	Layouts  map[string]Layout  `json:"layouts"`
	Features map[string]Feature `json:"features,omitempty"`
	// Fonts is the font files the template bundles, by an id local to the
	// template. A template that uses only the engine's built-in fonts has none
	Fonts map[string]FontFile `json:"fonts,omitempty"`
	// Roles is the text roles the template defines, by the name a text box uses.
	// The standard roles exist without an entry here, and an entry redefines one
	Roles map[string]FontRole `json:"roles,omitempty"`
}

// FontFile is a font a template bundles
type FontFile struct {
	// File is the font's path in the bundle, a .ttf or .otf directly in "font/"
	File string `json:"file"`
	// Family and Style are what the font is called, shown in a settings panel.
	// Empty reads them from the font's own name table
	Family string `json:"family,omitempty"`
	Style  string `json:"style,omitempty"`
	// SHA256 is the lowercase hex hash of the file. A file that does not match it
	// is refused
	SHA256 string `json:"sha256,omitempty"`
	// Source and License say where the font comes from and under what terms
	Source  string `json:"source,omitempty"`
	License string `json:"license,omitempty"`
}

// FontRole is a text role and the font it draws with. Exactly one of Font and
// Builtin is set
type FontRole struct {
	// Label is how a settings panel names the role. Empty is the role's name
	Label string `json:"label,omitempty"`
	// Font is a key of the template's Fonts
	Font string `json:"font,omitempty"`
	// Builtin is the id of a font that ships with the engine, such as
	// "mimic-m15-title"
	Builtin string `json:"builtin,omitempty"`
}

// Author is who made the template
type Author struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Credit is one person or source the template's art comes from
type Credit struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
	Note string `json:"note,omitempty"`
}

// Part is an art set: one image per material, and the slot that picks which
type Part struct {
	// Slot is the name of the frame slot the part reads, such as "pinlines". Empty
	// is no slot, so only the "any" image can draw
	Slot string `json:"slot,omitempty"`
	// Variants is the image for each material, by the material's key, such as "w",
	// "wg", "gold", or "any" for an image that does not depend on the card
	Variants map[string]Variant `json:"variants"`
	// Origin is where the images' top left sits in a layout's canvas, since the
	// packer cuts each part down to the pixels it uses. Every variant shares it
	Origin Point `json:"origin,omitempty"`
	// Blend tunes how a pair of colors mixes across the part. A part that reads a
	// pair blends by default, and a pair image among the variants is used as it is
	Blend *Blend `json:"blend,omitempty"`
}

// Variant is one image of a part
type Variant struct {
	// Path is the image's place in the bundle, slash separated and relative to its
	// root
	Path string `json:"path"`
}

// Point is a position in a layout's canvas
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Blend describes how the colors of a run mix from left to right
type Blend struct {
	// Seams gives the transitions of a run, keyed by how many colors the run has,
	// "2", "3", or "4". A run of n colors has n-1 seams, one between each
	// neighbouring pair. A pair needs none and takes the engine's default, and a run
	// of three or four colors that has no entry here does not draw
	Seams map[string][]Seam `json:"seams,omitempty"`
	// Shape is "smooth" for a smoothstep ramp, or "linear". Empty is smooth
	Shape string `json:"shape,omitempty"`
	// End is the share of the right-hand color at the far right, from above 0 to
	// 1. The far left is always the left color. Zero is 1
	End float64 `json:"end,omitempty"`
}

// Seam is one transition between two neighbouring colors
type Seam struct {
	// Center is where the transition is centered, in pixels from the middle of the
	// canvas, for the part drawn at its own origin. An entry placed elsewhere with
	// at carries its seams along by the same distance
	Center float64 `json:"center"`
	// Width is how wide the transition is, in pixels
	Width float64 `json:"width"`
}

// Canvas is the size a layout is drawn at and how it is delivered
type Canvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// DPI is the resolution the canvas is authored at. Zero infers it from the
	// width
	DPI int `json:"dpi,omitempty"`
	// Rotate turns the finished card this many degrees clockwise, a multiple of
	// 90, so a frame authored on its side delivers upright
	Rotate int `json:"rotate,omitempty"`
}

// Layout is everything needed to draw one card shape
type Layout struct {
	// Serves lists the shapes the layout draws
	Serves    []frame.Shape      `json:"serves"`
	Canvas    Canvas             `json:"canvas"`
	Stack     []Entry            `json:"stack"`
	TextBoxes map[string]TextBox `json:"textBoxes"`
	// Art is the places the card's art goes
	Art     []ArtWindow       `json:"art,omitempty"`
	Symbols map[string]Symbol `json:"symbols,omitempty"`
}

// Entry is one use of a part in a layout's stack, which runs bottom to top
type Entry struct {
	// ID names the entry for a text box or an art window that refers to it. Empty
	// is the part's name
	ID   string `json:"id,omitempty"`
	Part string `json:"part"`
	// Condition is when the entry draws, from the engine's vocabulary
	Condition string `json:"condition,omitempty"`
	// Face is which face's data the entry reads: "self", the face being drawn and
	// the default, "first", "second", or "other"
	Face string `json:"face,omitempty"`
	// At places the image's top left in the canvas in place of the part's origin
	At *Point `json:"at,omitempty"`
	// Mirror flips a region of the entry left to right when a condition holds
	Mirror *Mirror `json:"mirror,omitempty"`
	// Mode is the compositing mode, such as "multiply". Empty is normal
	Mode string `json:"mode,omitempty"`
	// Opacity is from 0 to 1, and nil is 1. OpacityFeature names a range feature
	// to use in its place
	Opacity        *float64 `json:"opacity,omitempty"`
	OpacityFeature string   `json:"opacityFeature,omitempty"`
	// Rotate turns the image this many degrees clockwise, a multiple of 90
	Rotate int `json:"rotate,omitempty"`
}

// Key is the name the entry is referred to by
func (e Entry) Key() string {
	if e.ID != "" {
		return e.ID
	}
	return e.Part
}

// Mirror is a left to right flip about the canvas's vertical center. The region
// is on the left half, and each pixel in it trades places with its mirror image
// on the right. A zero width or height flips the whole image
type Mirror struct {
	Condition string `json:"condition,omitempty"`
	X         int    `json:"x,omitempty"`
	Y         int    `json:"y,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// ArtWindow is where the card's art goes
type ArtWindow struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// After is the ID of the stack entry the art sits directly above
	After string `json:"after"`
	// Face is which face's art the window shows, as for an entry
	Face   string `json:"face,omitempty"`
	Rotate int    `json:"rotate,omitempty"`
}

// Symbol is where a symbol draws. It arrives colored for the card's rarity, so
// the manifest only places it, scaled to fit inside the box without changing its
// shape
type Symbol struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// Align places the symbol across the box, "left", "center", or "right". Empty
	// is right
	Align string `json:"align,omitempty"`
	// VAlign places it down the box, "top", "center", or "bottom". Empty is center
	VAlign string `json:"vAlign,omitempty"`
	// Scale shrinks the fitted symbol to this fraction of the box. Zero is 1
	Scale     float64 `json:"scale,omitempty"`
	Condition string  `json:"condition,omitempty"`
	Face      string  `json:"face,omitempty"`
}

// TextBox is one box of text, named by its key in a layout. The key is what other
// boxes refer to, and what the box draws is its Draws, which is the key when it
// has none. A split card has two boxes that draw the title, one for each half
type TextBox struct {
	// Draws names what the engine fills into the box, such as "title" or "oracle"
	Draws string    `json:"draws,omitempty"`
	Rect  Rect      `json:"rect"`
	Text  TextStyle `json:"text"`
	Flow  Flow      `json:"flow,omitempty"`
	// Condition is when the box draws
	Condition string `json:"condition,omitempty"`
	Face      string `json:"face,omitempty"`
	// Space is "output" for a box laid out after the layout's rotation, in the
	// delivered canvas's own coordinates. Empty is the authored canvas
	Space  string `json:"space,omitempty"`
	Rotate int    `json:"rotate,omitempty"`
	// When lists overrides. The first whose condition holds, trying those that
	// name the most conditions first, is merged over the box
	When []Override `json:"when,omitempty"`
}

// Content is what the box draws
func (b TextBox) Content(key string) string {
	if b.Draws != "" {
		return b.Draws
	}
	return key
}

// Rect is a position and size
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TextStyle is how a box's text looks
type TextStyle struct {
	FontSize float64 `json:"fontSize"`
	// Font is the font role: one of the standard ones, "title", "body",
	// "body-italic", "mana", "small-caps", or "info", or one the template defines
	// in Roles. Empty is body
	Font  string `json:"font,omitempty"`
	Color string `json:"color,omitempty"`
	// Align is "left", "center", or "right". Empty is left
	Align string `json:"align,omitempty"`
	// VAlign anchors the text block vertically: "top", "center", "bottom", or one of
	// the single-line anchors that set the box's Y on the first line, "baseline" for its
	// baseline, "cap-top" for the top of its capital letters and "cap-center" for their
	// middle. Those three shrink to fit the width. Empty is top
	VAlign string `json:"vAlign,omitempty"`
	// LineSpacing is the baseline to baseline distance as a multiple of the font
	// size, so 1 is solid. Zero is the face's natural line height
	LineSpacing float64 `json:"lineSpacing,omitempty"`
	// MinFontSize is the floor for shrinking text to fit. Zero is a default fraction
	// of FontSize
	MinFontSize float64 `json:"minFontSize,omitempty"`
	// PaddingX and PaddingY inset the text from the box edge on each axis
	PaddingX int `json:"paddingX,omitempty"`
	PaddingY int `json:"paddingY,omitempty"`
	// Tracking is letter spacing in thousandths of an em
	Tracking float64 `json:"tracking,omitempty"`
	Shadow   *Shadow `json:"shadow,omitempty"`
}

// Shadow is a solid offset copy of a box's ink drawn behind it
type Shadow struct {
	// Distance is how far the shadow sits from the ink, as a fraction of the font size
	Distance float64 `json:"distance"`
	// Angle is its direction, in degrees clockwise from straight down
	Angle float64 `json:"angle,omitempty"`
	// Opacity is from above 0 to 1. Zero is 1
	Opacity float64 `json:"opacity,omitempty"`
	// Color is "#RRGGBB". Empty is black
	Color string `json:"color,omitempty"`
}

// Flow is how a box makes room for the other things on the card
type Flow struct {
	// ClearOf names a text box this one narrows to stay clear of, such as a name
	// giving way to its mana cost. ClearGap is the gap kept, as a fraction of the
	// font size, and zero is half
	ClearOf  string  `json:"clearOf,omitempty"`
	ClearGap float64 `json:"clearGap,omitempty"`
	// AvoidLayer names the ID of a stack entry the box's text keeps out of, such as
	// the P/T box. It only narrows for a card whose render draws that entry
	AvoidLayer string `json:"avoidLayer,omitempty"`
	// Duck moves the box to another box's row when an entry draws
	Duck *Duck `json:"duck,omitempty"`
	// Divider names the part drawn between rules text and flavor text
	Divider string `json:"divider,omitempty"`
}

// Duck moves a box to the row of another text box when a stack entry draws, such
// as a copyright line ducking under the artist row on a creature
type Duck struct {
	Layer string `json:"layer"`
	Row   string `json:"row"`
}

// Override is a condition and the changes it makes to a text box. The changes are
// written as the parts of a TextBox they replace, so one that moves a box when a
// color indicator is present says only what moves
type Override struct {
	Condition string
	// Patch is the rest of the override, a JSON object merged over the box
	Patch json.RawMessage
	// Box is the text box with the patch merged over it, filled in when the
	// manifest is decoded
	Box *TextBox
}

// Feature is a choice the user makes
type Feature struct {
	// Type is "bool", "choice", or "range"
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
	// Values lists the options of a choice
	Values []string `json:"values,omitempty"`
	// Min, Max, and Step bound a range
	Min  float64 `json:"min,omitempty"`
	Max  float64 `json:"max,omitempty"`
	Step float64 `json:"step,omitempty"`
	// Default is a bool for a bool feature, one of Values for a choice, and a
	// number in range for a range. Absent is false, the first value, and Min
	Default json.RawMessage `json:"default,omitempty"`
}

// MarshalJSON writes an override as one object with its condition first
func (o Override) MarshalJSON() ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if len(o.Patch) > 0 {
		if err := json.Unmarshal(o.Patch, &fields); err != nil {
			return nil, err
		}
	}
	cond, err := json.Marshal(o.Condition)
	if err != nil {
		return nil, err
	}
	fields["condition"] = cond
	return json.Marshal(fields)
}

// UnmarshalJSON reads an override, taking its condition and keeping the rest as
// the patch
func (o *Override) UnmarshalJSON(data []byte) error {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*o = Override{}
	if raw, ok := fields["condition"]; ok {
		if err := json.Unmarshal(raw, &o.Condition); err != nil {
			return err
		}
		delete(fields, "condition")
	}
	patch, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	o.Patch = patch
	return nil
}
