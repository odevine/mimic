package svgpath

import (
	"github.com/odevine/impasto/path"
	"github.com/odevine/impasto/raster"
)

// Draw paints the icon into dst, mapping the icon's coordinates to dst's pixels
// through m. Each shape is filled and then stroked, in document order, over
// what is already in dst. A stroke's width follows the scale of m and of the
// shape's own transform
func (ic *Icon) Draw(dst *raster.Buffer, m Affine) {
	for _, s := range ic.Shapes {
		s.draw(dst, m)
	}
}

func (s Shape) draw(dst *raster.Buffer, m Affine) {
	dev := s.outline.buildAffine(m)
	local := m.Mul(s.ctm)
	if s.fill != nil {
		path.Fill(dst, dev, s.Rule, s.fill.src.paint(local, s.fill.alpha))
	}
	if s.stroke != nil {
		k := local.Scale()
		style := path.StrokeStyle{
			Width:      float32(s.stroke.width * k),
			Cap:        s.stroke.cap,
			Join:       s.stroke.join,
			MiterLimit: float32(s.stroke.miter),
			DashOffset: float32(s.stroke.dashOffset * k),
		}
		for _, d := range s.stroke.dash {
			style.Dash = append(style.Dash, float32(d*k))
		}
		path.StrokePath(dst, dev, style, s.stroke.src.paint(local, s.stroke.alpha))
	}
}
