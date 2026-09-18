package template

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// RenderTextBox draws greedily word-wrapped text into a document-sized
// transparent image, positioned and aligned within box, and returns it to be
// composited like any other layer.
//
// The face parameter is where a caller supplies a font they have obtained
// themselves. This module bundles none. The current handling has no shaping
// beyond what the face provides and no shrink-to-fit: text that overflows the
// box height is dropped rather than resized, and a braced mana symbol in rules
// text renders as its literal characters
func RenderTextBox(box TextBoxSpec, text string, docW, docH int, face font.Face) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, docW, docH))
	if strings.TrimSpace(text) == "" {
		return img
	}

	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(parseHexColor(box.Color)),
		Face: face,
	}
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = metrics.Ascent.Ceil() + metrics.Descent.Ceil()
	}
	baseline := box.Y + metrics.Ascent.Ceil()
	bottom := box.Y + box.Height
	space := drawer.MeasureString(" ")

	drawLine := func(words []string, width fixed.Int26_6) {
		if len(words) == 0 || baseline > bottom {
			return
		}
		x := box.X
		switch box.Align {
		case "center":
			x = box.X + (box.Width-width.Ceil())/2
		case "right":
			x = box.X + box.Width - width.Ceil()
		}
		drawer.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(baseline)}
		drawer.DrawString(strings.Join(words, " "))
		baseline += lineHeight
	}

	for _, para := range strings.Split(text, "\n") {
		var line []string
		var width fixed.Int26_6
		for _, word := range strings.Fields(para) {
			advance := drawer.MeasureString(word)
			switch {
			case len(line) == 0:
				line = []string{word}
				width = advance
			case (width + space + advance).Ceil() <= box.Width:
				line = append(line, word)
				width += space + advance
			default:
				drawLine(line, width)
				line = []string{word}
				width = advance
			}
		}
		drawLine(line, width)
		if baseline > bottom {
			break
		}
	}
	return img
}

// parseHexColor reads a "#RRGGBB" string, returning opaque black when it cannot
func parseHexColor(s string) color.Color {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.Black
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.Black
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}
