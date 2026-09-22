package template

import (
	"math"

	"github.com/odevine/impasto/effects"
)

// ShadowSpec is a solid offset copy of a box's ink, drawn behind it the way a
// printed card's mana cost casts a hard shadow rather than a soft one
type ShadowSpec struct {
	// Distance is how far the shadow sits from the ink, as a fraction of the
	// box's own FontSize
	Distance float64 `json:"distance"`
	// Angle is the shadow's direction, in degrees clockwise from straight down
	Angle float64 `json:"angle,omitempty"`
	// Opacity is 0 to 1. Zero or less defaults to fully opaque
	Opacity float64 `json:"opacity,omitempty"`
	// Color is "#RRGGBB". Empty defaults to black
	Color string `json:"color,omitempty"`
}

// Effect builds the drop shadow this spec describes for a box drawn at
// fontSize
func (s ShadowSpec) Effect(fontSize float64) *effects.DropShadow {
	opacity := s.Opacity
	if opacity <= 0 {
		opacity = 1
	}
	// impasto measures the angle clockwise from the positive x axis, so half
	// pi is straight down and Angle turns it further clockwise from there
	angle := math.Pi/2 + s.Angle*math.Pi/180
	return &effects.DropShadow{
		Color:    ParseHexColor(s.Color),
		Opacity:  float32(opacity),
		Angle:    float32(angle),
		Distance: float32(fontSize * s.Distance),
	}
}
