package template

import "io"

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
	// Condition is "", "legendary", "nonlegendary", "land", "nonland", or a
	// color key. Its vocabulary is defined by the template that reads it
	Condition string `json:"condition,omitempty"`
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
}

// ArtSlot is where the card's art goes and which layer it sits directly above
type ArtSlot struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	After  string `json:"after"` // insert art directly above the LayerSpec with this Name
}
