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
// the fonts and roles tables, and minor 2 a layout's sections
const Contract = "1.2"

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
	// Sections are the regions whose rules text the engine splits into rows, one
	// for each loyalty ability, saga chapter, or class level, by a name local to
	// the layout
	Sections map[string]Section `json:"sections,omitempty"`
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
	// After is the ID of the stack entry the art sits directly above. Empty puts
	// the art beneath every entry, for a frame that is translucent over it
	After string `json:"after,omitempty"`
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
	// middle. Those three shrink to fit the width, and measure the capitals from the
	// first text face on the line. Empty is top
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
	// Prefix is literal text drawn before this box's own value, in the same
	// style, such as the "Illus. " an old-bordered card prints before its
	// artist credit in place of the modern frame's nib icon. The artist
	// box's nib icon does not draw when a Prefix is set
	Prefix string `json:"prefix,omitempty"`
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

// Section is a region of a layout whose rules text the engine splits into rows
// and spreads from top to bottom, a row for each ability, chapter, or level the
// card prints. The rows' text shrinks together until every row fits, and each
// row can carry images and text beside it, such as a loyalty cost's badge or a
// chapter's number, which move with the row
type Section struct {
	// Split is how the rules text splits into rows: "loyalty" for a
	// planeswalker's abilities, "chapters" for a saga's, or "levels" for a
	// class's. A line the split does not recognize, such as a static ability, is
	// a row with no cost or chapter
	Split string `json:"split"`
	// Rect is the region the rows spread over
	Rect Rect `json:"rect"`
	// Text is how each row's rules text looks, and the size it starts from before
	// the rows shrink to fit
	Text TextStyle `json:"text"`
	// Body is the column a row's text sits in, which is the rect's width when
	// unset. Plain is the column of a row that has nothing for its split's key
	// field, such as a static ability, when that sits apart from the others
	Body  *Span `json:"body,omitempty"`
	Plain *Span `json:"plain,omitempty"`
	// Gap is the least space kept between rows and at the region's top and bottom
	// before the text shrinks, in pixels
	Gap int `json:"gap,omitempty"`
	// Spread shares out the space the rows leave
	Spread *Spread `json:"spread,omitempty"`
	// Header reserves room above each row that has a field, such as the level bar
	// over each of a class's levels after the first
	Header *Header `json:"header,omitempty"`
	// Items are the images and text drawn beside each row
	Items []RowItem `json:"items,omitempty"`
	// Divider is an image drawn in the middle of the space between two rows
	Divider *RowPart `json:"divider,omitempty"`
	// Bands shade every other row, such as a planeswalker's alternate abilities
	Bands *Bands `json:"bands,omitempty"`
	// AvoidLayer names the ID of a stack entry the rows' text keeps out of, such
	// as a loyalty box, for a card whose render draws it
	AvoidLayer string `json:"avoidLayer,omitempty"`
	// Above is the ID of the stack entry the section's images sit directly above.
	// Empty draws them above the whole stack. Text always draws on top
	Above     string `json:"above,omitempty"`
	Condition string `json:"condition,omitempty"`
	Face      string `json:"face,omitempty"`
}

// Span is a horizontal stretch of the canvas
type Span struct {
	X     int `json:"x"`
	Width int `json:"width"`
}

// Spread weighs how the space the rows leave is shared. Outside is the share of
// the space above the first row and of the space below the last, and Inside the
// share of each space between two rows. Zero is 1 for both, which spaces the
// rows evenly
type Spread struct {
	Outside float64 `json:"outside,omitempty"`
	Inside  float64 `json:"inside,omitempty"`
}

// Header is room held above each row that has a field
type Header struct {
	// For is the row field a row needs to have a header: "cost", "chapter", or
	// "level"
	For    string `json:"for"`
	Height int    `json:"height"`
}

// RowItem is an image or a text drawn beside each row, authored in place for one
// row and moved with each. Y is where the anchor of that authored row was, so a
// row whose anchor is 100 pixels lower draws the item 100 pixels lower
type RowItem struct {
	// For is the row field the item shows: "cost", "chapter", or "level". The item
	// draws only for a row that has it, once for each value it holds. Empty draws
	// for every row
	For string `json:"for,omitempty"`
	// Part is the image the item draws. Parts picks one by the key of the row's
	// value in place of it: a loyalty cost's sign, "plus", "minus", or "zero", or
	// the value itself, such as a chapter's "II". A row whose key has no image
	// draws Part, or nothing when there is none
	Part  string            `json:"part,omitempty"`
	Parts map[string]string `json:"parts,omitempty"`
	// Rect and Text draw the row's value as text in place of an image. Format
	// wraps the value, with {} standing for it, such as "Level {}" or "{}:", and
	// one with no {} prints as written, such as a planeswalker's colon
	Rect   *Rect      `json:"rect,omitempty"`
	Text   *TextStyle `json:"text,omitempty"`
	Format string     `json:"format,omitempty"`
	// Anchor is the point of a row the item moves with: "center", the middle of
	// the row's text and the default, "top", the top of the row with its header,
	// or "first-line", the middle of the row's opening line alone, for an item
	// such as a planeswalker's loyalty badge that is authored beside one line
	// and should not drift toward the middle of a row that wraps further
	Anchor string `json:"anchor,omitempty"`
	Y      int    `json:"y"`
	// Step is how far apart the copies of an item for a row with several values
	// sit, centered on the anchor as a group
	Step int `json:"step,omitempty"`
}

// RowPart is an image placed between rows. Y is the point of the image as
// authored that lands on the place it goes
type RowPart struct {
	Part string `json:"part"`
	Y    int    `json:"y"`
}

// Bands shade alternate rows with an image cut to each row's band, which runs
// from the middle of the space above the row to the middle of the space below,
// or to the image's own edge for the first and last row
type Bands struct {
	Part string `json:"part"`
	// Rows is which rows are shaded: "odd" for the second, fourth, and so on, the
	// default, or "even" for the first, third, and so on
	Rows string `json:"rows,omitempty"`
	// Top and Bottom are images drawn at a band's top and bottom edges, such as a
	// ragged line, where those edges fall between two rows
	Top    *RowPart `json:"top,omitempty"`
	Bottom *RowPart `json:"bottom,omitempty"`
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
