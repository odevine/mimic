package template

import (
	"image"
	"io"
)

// AssetProvider supplies the two things impasto has no opinion about: the bytes
// of a template's resolved manifest, and where its images live. Keeping it an
// interface means a filesystem layout is not baked into every template, so a
// directory, a bundle, or an in-memory set can stand in without any template
// code changing
type AssetProvider interface {
	// ReadManifest returns the bytes of the template's resolved manifest. Load
	// decodes and checks them
	ReadManifest() ([]byte, error)
	// Open returns a reader for the image at a slash separated path relative to
	// the template's root, and fails for one that would leave it
	Open(relPath string) (io.ReadCloser, error)
}

// TextBoxSpec is where and how one text box draws, in the flat form the text layout
// reads. A layout compiles each of its manifest text boxes into one
type TextBoxSpec struct {
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FontSize float64 `json:"fontSize"`
	Align    string  `json:"align"` // "left" | "center" | "right"
	Color    string  `json:"color"` // "#RRGGBB"
	// VAlign anchors the text block vertically: "top", "center", "bottom", or one of
	// "baseline", "cap-top" and "cap-center", which set Y on the first line of point text,
	// at its baseline, the top of its capitals or their middle. The capitals are those of
	// the first text face on the line. Empty means top
	VAlign string `json:"vAlign,omitempty"`
	// LineSpacing sets the baseline-to-baseline distance to this multiple of the
	// font size, the way leading reads in a PSD, so 1.0 is solid. Zero or less
	// means the face's natural line height
	LineSpacing float64 `json:"lineSpacing,omitempty"`
	// MinFontSize is the floor for shrink-to-fit on an area box. Text that still
	// overflows at this size is clipped. Zero means a default fraction of
	// FontSize. A point text box shrinks to its width and not its height, so it ignores this
	MinFontSize float64 `json:"minFontSize,omitempty"`
	// Padding insets the text from the box edge on every side, so the box can
	// stay the size of the visible panel while the text keeps a margin. Zero
	// draws the text to the edge
	Padding int `json:"padding,omitempty"`
	// PaddingX and PaddingY override Padding on one axis, so a box can hold a
	// side margin without a top and bottom one. Nil takes Padding, and a set
	// zero really is no inset on that axis
	PaddingX *int `json:"paddingX,omitempty"`
	PaddingY *int `json:"paddingY,omitempty"`
	// Avoid is a rectangle inside the box that text keeps out of, in document
	// coordinates. A line whose ink would cross it wraps to stop at its left
	// edge instead, so a creature's rules text runs the full height of its box
	// and steps around the P/T box rather than stopping above it. It narrows a
	// line from the right, which is the corner a P/T box sits in. The zero
	// rectangle keeps nothing out, and the manifest does not carry it since the
	// P/T box's place comes from its art
	Avoid image.Rectangle `json:"-"`
	// Tracking is letter spacing in Photoshop's thousandths of an em, so 125
	// adds 0.125em after each glyph. Zero draws with the face's own advances.
	// It applies to the drawn pen and to each token's measured width
	Tracking float64 `json:"tracking,omitempty"`
	// Font names the font role this box draws in: "title", "body",
	// "body-italic", "mana", "small-caps", or "info" (see engine/fonts).
	// Empty, or a name the engine does not recognize, draws in the body role
	Font string `json:"font,omitempty"`
	// Prefix is literal text drawn before this box's own value, in the same
	// style, such as the "Illus. " an old-bordered card prints before its
	// artist credit in place of the modern frame's nib icon. Empty draws
	// nothing extra, and the artist box's nib icon does not draw when a
	// Prefix is set, since the prefix already labels the credit
	Prefix string `json:"prefix,omitempty"`
	// AvoidLayer names a layer this box's ink keeps clear of, the way a
	// creature's rules text steps around its P/T box. The engine resolves
	// the named layer the same way the main compositing pass would (its own
	// Condition and ColorSlot), so a box asking to avoid a layer that does
	// not render for this card gets its full room back
	AvoidLayer string `json:"avoidLayer,omitempty"`
	// ClearOf names another box this one narrows to stay clear of, the way a
	// card's name gives way to its mana cost. It only ever narrows, so a
	// card whose named box leaves room keeps the width the manifest gave it
	ClearOf string `json:"clearOf,omitempty"`
	// ClearGap is the gap kept from the box named by ClearOf, as a fraction
	// of this box's own FontSize. Zero or less defaults to 0.5
	ClearGap float64 `json:"clearGap,omitempty"`
	// Shadow, when set, draws a solid offset copy of this box's ink behind
	// it, the way a printed card's mana cost casts a hard shadow rather than
	// a soft one. A box with no Shadow draws no shadow at all
	Shadow *ShadowSpec `json:"shadow,omitempty"`
	// DuckLayer names a layer whose presence for this card, resolved the
	// same way AvoidLayer is, moves this box to the row named by DuckRow,
	// the way a copyright line ducks under the artist row on a creature
	// whose P/T box would otherwise collide with it in the collector row.
	// Both must be set for either to take effect
	DuckLayer string `json:"duckLayer,omitempty"`
	DuckRow   string `json:"duckRow,omitempty"`
	// Space is "output" for a box laid out after the manifest's Rotate, in the
	// delivered canvas's own coordinates, such as a legal line that reads upright
	// on a card turned from its authored side. Empty is the authored canvas
	Space string `json:"space,omitempty"`
	// Rotate turns the box's text clockwise, a multiple of 90. The rect is where the
	// box ends up, so the text is laid out in it turned a quarter for 90 or 270
	Rotate int `json:"rotate,omitempty"`
}

// ArtSlot is where the card's art goes and which layer it sits directly above
type ArtSlot struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	After  string `json:"after"` // insert art directly above the LayerSpec with this Name
	// Rotate turns the art clockwise within the window, a multiple of 90
	Rotate int `json:"rotate,omitempty"`
}

// SpaceOutput is the TextBoxSpec.Space of a box laid out after the rotation
const SpaceOutput = "output"

// SymbolSet is the key of the set expansion symbol in a layout's symbols
const SymbolSet = "set"

// SymbolSpec is where a symbol draws. The symbol arrives colored for the card's
// rarity, so the manifest only places it: it is scaled to fit inside the box
// without changing its shape, then placed by Align and VAlign
type SymbolSpec struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// Align places the symbol across the box, "left", "center" or "right". Empty
	// means right, where a set symbol ends the type line
	Align string `json:"align,omitempty"`
	// VAlign places it down the box, "top", "center" or "bottom". Empty means
	// center
	VAlign string `json:"vAlign,omitempty"`
	// Scale shrinks the fitted symbol to this fraction of the box. Zero or less
	// means 1
	Scale float64 `json:"scale,omitempty"`
}
