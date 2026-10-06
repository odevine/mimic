package template

import (
	"image"
	"io"
)

// AssetProvider supplies the two things impasto has no opinion about: where a
// template's layer PNGs live, and where things go on the canvas. Keeping it an
// interface means a filesystem layout is not baked into every template, so an
// embed.FS-backed or remote-bundle-backed provider can stand in later without
// any template code changing
type AssetProvider interface {
	Manifest() (*Manifest, error)
	Open(relPath string) (io.ReadCloser, error)
}

// Manifest ties a template's PNG layers to a layout. Its JSON tags are the
// on-disk manifest.json schema an FSAssetProvider reads
type Manifest struct {
	Template  string                 `json:"template"`
	Width     int                    `json:"width"`
	Height    int                    `json:"height"`
	DPI       int                    `json:"dpi,omitempty"` // zero infers from width
	Layers    []LayerSpec            `json:"layers"`        // bottom to top
	TextBoxes map[string]TextBoxSpec `json:"textBoxes"`
	// Art is the one art window of a single-faced frame. A manifest with Arts
	// leaves it empty
	Art ArtSlot `json:"art"`
	// Arts is one art window per half of a split card, first half first. The
	// card's one art image is cut in half, left then right, one half to each
	Arts []ArtSlot `json:"arts,omitempty"`
	// Symbols are the places a card's set symbol can draw, keyed by name. Only "set"
	// is drawn today. A manifest without it draws no symbol
	Symbols map[string]SymbolSpec `json:"symbols,omitempty"`
	// Rotate turns the finished composite this many degrees clockwise, a multiple
	// of 90, so a frame authored on its side delivers upright. Everything above
	// is laid out in the authored canvas, and only a TextBoxSpec with Space
	// "output" is laid out after the turn
	Rotate int `json:"rotate,omitempty"`
}

// LayerSpec is one frame layer. Layers are an ordered slice, not a map,
// because z-order matters and a map has none. The order is bottom to top,
// matching the order layers are appended into a canvas group
type LayerSpec struct {
	Name string `json:"name"`
	// Condition is one of the engine's fixed vocabulary: "", "legendary",
	// "nonlegendary", "land", "nonland", "creature", "color_indicator",
	// "front", "back", "fuse", "set_symbol" (a set symbol draws for the card),
	// "icon_left", "icon_right" (which side of the title bar
	// the transform icon sits on), or a value the engine does not yet drive (nyx,
	// companion, hollow_crown, fullart, divider, pt_dark), which renders the
	// layer off. A comma-separated list such as "back,land" holds when every
	// entry does
	Condition string `json:"condition,omitempty"`
	// ColorSlot names which of a WUBRG frame's slots (background, pinlines,
	// twins, ptBox, crown, indicator, transform_icon, fuse) this layer's color key
	// comes from, see engine/frame. Empty means the layer carries only a
	// color-invariant "any" variant, such as a border or a divider
	ColorSlot string `json:"colorSlot,omitempty"`
	// ColorVariants is keyed by color key ("w", "gold", "land", ...). The key
	// "any" is the color-invariant fallback
	ColorVariants map[string]LayerAsset `json:"colorVariants"`
	Blend         string                `json:"blend,omitempty"` // blend.Mode name, default "normal"
	// Mirror flips part of the layer left to right when its condition holds,
	// so one cut can draw a frame whose icon sits on either end of the title bar
	Mirror *LayerMirror `json:"mirror,omitempty"`
	// ColorBlend lets a layer whose color key is several color letters, such as
	// "rw", draw each letter's own variant across the layer when no variant is
	// keyed by the whole key. The colors follow one another from left to right in
	// the authored canvas, blending where BlendStops puts the seams
	ColorBlend bool `json:"colorBlend,omitempty"`
	// Half scopes the layer to one half of a split card, 1 for the first and 2 for
	// the second, so its condition and color slot read that half's own face. Zero
	// is the whole card, which is every layer of a single-faced frame
	Half int `json:"half,omitempty"`
	// X and Y place the layer's PNG in the document, so one cut smaller than the
	// document can draw at either half. Zero places it at the origin, where a
	// document-sized PNG belongs
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

// LayerMirror is a left to right flip of a layer about the canvas's vertical
// center. The region is given on the left half in document coordinates, and
// each pixel in it trades places with its mirror image on the right, so the
// rest of the layer keeps its own art. A zero width or height flips the whole
// layer
type LayerMirror struct {
	Condition string `json:"condition,omitempty"`
	X         int    `json:"x,omitempty"`
	Y         int    `json:"y,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// Region is the mirror's rectangle, or the empty rectangle for the whole layer
func (m LayerMirror) Region() image.Rectangle {
	if m.Width <= 0 || m.Height <= 0 {
		return image.Rectangle{}
	}
	return image.Rect(m.X, m.Y, m.X+m.Width, m.Y+m.Height)
}

// LayerAsset points at one PNG within the provider's root
type LayerAsset struct {
	Path string `json:"path"`
}

// TextBoxSpec is where and how one named text box draws
type TextBoxSpec struct {
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FontSize float64 `json:"fontSize"`
	Align    string  `json:"align"` // "left" | "center" | "right"
	Color    string  `json:"color"` // "#RRGGBB"
	// VAlign anchors the text block vertically: "top", "center", "bottom", or
	// "baseline", which sets Y as the first line's baseline for point text.
	// Empty means top
	VAlign string `json:"vAlign,omitempty"`
	// LineSpacing sets the baseline-to-baseline distance to this multiple of the
	// font size, the way leading reads in a PSD, so 1.0 is solid. Zero or less
	// means the face's natural line height
	LineSpacing float64 `json:"lineSpacing,omitempty"`
	// MinFontSize is the floor for shrink-to-fit on an area box. Text that still
	// overflows at this size is clipped. Zero means a default fraction of
	// FontSize. A baseline-anchored box does not shrink, so it ignores this
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
	// Box names the logical box this spec fills, such as "title", so several
	// specs can offer one box under different conditions. Empty means the
	// spec's own key in TextBoxes
	Box string `json:"box,omitempty"`
	// Condition is a layer condition (see LayerSpec.Condition) that must hold
	// for this spec to fill its box. Of the specs filling one box, the one
	// drawn is the first whose condition holds, trying those naming the most
	// conditions first and then in sorted key order, so a spec with no
	// condition is the fallback when none of the others hold
	Condition string `json:"condition,omitempty"`
	// Half scopes the box to one half of a split card, as LayerSpec.Half does, so
	// it draws that face's text and resolves its condition against that face
	Half int `json:"half,omitempty"`
	// Space is "output" for a box laid out after the manifest's Rotate, in the
	// delivered canvas's own coordinates, such as a legal line that reads upright
	// on a card turned from its authored side. Empty is the authored canvas
	Space string `json:"space,omitempty"`
}

// ArtSlot is where the card's art goes and which layer it sits directly above
type ArtSlot struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	After  string `json:"after"` // insert art directly above the LayerSpec with this Name
}

// SpaceOutput is the TextBoxSpec.Space of a box laid out after the rotation
const SpaceOutput = "output"

// ArtSlots is the art windows of the manifest in order, the single Art or each
// of Arts
func (m *Manifest) ArtSlots() []ArtSlot {
	if len(m.Arts) > 0 {
		return m.Arts
	}
	return []ArtSlot{m.Art}
}

// SymbolSet is the key of the set expansion symbol in Manifest.Symbols
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
	// Condition is a layer condition (see LayerSpec.Condition) that must hold for
	// the symbol to draw
	Condition string `json:"condition,omitempty"`
}
