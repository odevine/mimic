package main

import (
	"image"
	"image/png"
	"net/http"
	"strconv"
	"sync"

	"github.com/odevine/mimic/engine/mana"
)

// symbolMaxPx bounds the pip size a client can ask for, so a crafted request
// cannot make the server rasterize an enormous disc
const symbolMaxPx = 128

// symbols lazily builds the pip renderer from the engine's embedded mana font.
// It is the same renderer the templates draw costs with, so a pip under an
// input is the pip the card will print
var symbols = sync.OnceValue(func() *mana.Symbols { return mana.NewSymbols("") })

// handleSymbol draws one braced code as a PNG pip, answering 404 for a code the
// engine does not know, which is how the editor flags an unknown symbol
func (s *server) handleSymbol(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	px, err := strconv.Atoi(r.URL.Query().Get("px"))
	if err != nil || px < 8 {
		px = 32
	}
	px = min(px, symbolMaxPx)

	sym := symbols()
	if sym == nil {
		http.Error(w, "no mana font available", http.StatusNotFound)
		return
	}
	// Symbol reports the room a code takes at a text size, and a pip's disc is
	// PipDiameter of that size, so this asks for the text size that yields px
	if _, ok := sym.Symbol(code, float64(px)/mana.PipDiameter); !ok {
		http.Error(w, "unknown symbol", http.StatusNotFound)
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if err := sym.DrawSymbol(img, code, img.Bounds()); err != nil {
		http.Error(w, "drawing symbol: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_ = png.Encode(w, img)
}
