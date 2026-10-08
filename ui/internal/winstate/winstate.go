// Package winstate decides where a saved window goes. It knows nothing about the
// windowing system, so it can be tested without one
package winstate

// Rect is a rectangle in screen coordinates
type Rect struct{ X, Y, Width, Height int }

// Size is a width and a height
type Size struct{ Width, Height int }

// grab is how much of the window's top edge has to sit on a screen for the title
// bar to be reachable: enough width to click and a title bar's height to drag
const (
	grabWidth  = 120
	grabHeight = 32
)

// Fit returns the rectangle to open a window at, given where it was saved, the
// displays now connected, the size to use when there is nothing to restore, and
// the smallest size the window allows. It reports false when the window should
// open centered at the default size instead: nothing was saved, or the saved
// position no longer reaches any display, such as after unplugging a monitor.
// A saved size is kept inside the smallest size and the display it lands on
func Fit(saved *Rect, screens []Rect, min Size) (Rect, bool) {
	if saved == nil || saved.Width <= 0 || saved.Height <= 0 {
		return Rect{}, false
	}
	for _, s := range screens {
		if !reaches(*saved, s) {
			continue
		}
		r := *saved
		r.Width = clamp(r.Width, min.Width, s.Width)
		r.Height = clamp(r.Height, min.Height, s.Height)
		// A window wider than the display it was saved on is pulled back inside
		if r.X+r.Width > s.X+s.Width {
			r.X = s.X + s.Width - r.Width
		}
		if r.X < s.X {
			r.X = s.X
		}
		if r.Y+r.Height > s.Y+s.Height {
			r.Y = s.Y + s.Height - r.Height
		}
		if r.Y < s.Y {
			r.Y = s.Y
		}
		return r, true
	}
	return Rect{}, false
}

// reaches reports whether enough of the window's title bar is on the screen to
// grab it
func reaches(w, s Rect) bool {
	left, right := max(w.X, s.X), min(w.X+w.Width, s.X+s.Width)
	top := max(w.Y, s.Y)
	visibleWidth := right - left
	return visibleWidth >= min(grabWidth, w.Width) && top+grabHeight <= s.Y+s.Height && w.Y+grabHeight > s.Y && w.Y < s.Y+s.Height
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	return max(lo, min(v, hi))
}
