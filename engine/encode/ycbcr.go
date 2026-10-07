// Package encode turns a finished render into the pixels an image encoder
// takes. ToYCbCr goes from the compositor's float buffer straight to the
// 4:2:0 planes a JPEG stores, so a card never passes through an 8-bit RGB
// image on the way
package encode

import (
	"image"

	"github.com/odevine/impasto/raster"
)

// srgbCodes maps a linear light value, quantized to 16 bits, to its 8-bit sRGB
// code. 64 KiB stays resident in cache across a batch
var srgbCodes [65536]uint8

func init() {
	for i := range srgbCodes {
		srgbCodes[i] = uint8(raster.LinearToSRGB(float32(i)/65535)*255 + 0.5)
	}
}

// code is the 8-bit sRGB code of a linear value, clamped to [0,1]
func code(v float32) int32 {
	if !(v > 0) {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return int32(srgbCodes[int(v*65535+0.5)])
}

// ToYCbCr converts buf to 8-bit sRGB in the full-range BT.601 space JPEG uses,
// with each chroma sample averaged over its 2x2 block of pixels. Color is read
// as it is stored, premultiplied, which is the image composited over black, and
// alpha is dropped. The result is not dithered
func ToYCbCr(buf *raster.Buffer) *image.YCbCr {
	w, h := buf.Width, buf.Height
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x += 2 {
			// A block hanging off the right or bottom edge reads its inside
			// neighbor again, which averages the pixels that exist
			var sr, sg, sb int32
			for _, at := range [4]image.Point{{x, y}, {min(x+1, w-1), y}, {x, min(y+1, h-1)}, {min(x+1, w-1), min(y+1, h-1)}} {
				i := (at.Y*w + at.X) * 4
				r, g, b := code(buf.Pix[i]), code(buf.Pix[i+1]), code(buf.Pix[i+2])
				dst.Y[at.Y*dst.YStride+at.X] = uint8((19595*r + 38470*g + 7471*b + 1<<15) >> 16)
				sr, sg, sb = sr+r, sg+g, sb+b
			}
			c := y/2*dst.CStride + x/2
			dst.Cb[c] = chroma(-11059*sr - 21709*sg + 32768*sb)
			dst.Cr[c] = chroma(32768*sr - 27439*sg - 5329*sb)
		}
	}
	return dst
}

// chroma turns a fixed-point chroma sum over four pixels, scaled by 2^16, into a
// code centered on 128
func chroma(sum int32) uint8 {
	return uint8(min((128<<18+sum+1<<17)>>18, 255))
}
