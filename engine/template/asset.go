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
	Layers    []LayerSpec            `json:"layers"` // bottom to top
	TextBoxes map[string]TextBoxSpec `json:"textBoxes"`
	Art       ArtSlot                `json:"art"`
}

// LayerSpec is one frame layer. Layers are an ordered slice, not a map,
// because z-order matters and a map has none. The order is bottom to top,
// matching the order layers are appended into a canvas group
type LayerSpec struct {
	Name string `json:"name"`
	// Condition is one of the engine's fixed vocabulary: "", "legendary",
	// "nonlegendary", "land", "nonland", "creature", or a value the engine
	// does not yet drive (nyx, companion, hollow_crown, fullart,
	// color_indicator, divider, pt_dark), which renders the layer off
	Condition string `json:"condition,omitempty"`
	// ColorSlot names which of a WUBRG frame's slots (background, pinlines,
	// twins, ptBox, crown) this layer's color key comes from; see
	// engine/frame. Empty means the layer carries only a color-invariant
	// "any" variant, such as a border or a divider
	ColorSlot string `json:"colorSlot,omitempty"`
	// ColorVariants is keyed by color key ("w", "gold", "land", ...). The key
	// "any" is the color-invariant fallback
	ColorVariants map[string]LayerAsset `json:"colorVariants"`
	Blend         string                `json:"blend,omitempty"` // blend.Mode name, default "normal"
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
}

// ArtSlot is where the card's art goes and which layer it sits directly above
type ArtSlot struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	After  string `json:"after"` // insert art directly above the LayerSpec with this Name
}
