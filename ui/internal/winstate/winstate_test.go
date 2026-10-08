package winstate

import "testing"

var (
	main      = Rect{0, 0, 1920, 1080}
	left      = Rect{-1440, 0, 1440, 900}
	minimum   = Size{720, 520}
	oneScreen = []Rect{main}
	twoScreen = []Rect{main, left}
)

func TestNothingSavedOpensCentered(t *testing.T) {
	if _, ok := Fit(nil, oneScreen, minimum); ok {
		t.Error("a missing saved window was restored")
	}
	if _, ok := Fit(&Rect{10, 10, 0, 0}, oneScreen, minimum); ok {
		t.Error("a saved window with no size was restored")
	}
}

func TestSavedWindowOnADisplayIsRestored(t *testing.T) {
	got, ok := Fit(&Rect{200, 100, 1280, 820}, oneScreen, minimum)
	if !ok || got != (Rect{200, 100, 1280, 820}) {
		t.Errorf("Fit = %+v, %v, want the saved rectangle", got, ok)
	}
}

func TestWindowOnAnUnpluggedDisplayIsDropped(t *testing.T) {
	saved := &Rect{-1300, 100, 1000, 700}
	if got, ok := Fit(saved, twoScreen, minimum); !ok || got.X != -1300 {
		t.Errorf("with the second display: %+v, %v", got, ok)
	}
	if _, ok := Fit(saved, oneScreen, minimum); ok {
		t.Error("a window on a display that is gone was restored")
	}
}

func TestWindowBarelyOnScreenIsDropped(t *testing.T) {
	// Only 40 pixels of its left edge reach the display
	if _, ok := Fit(&Rect{1880, 100, 1000, 700}, oneScreen, minimum); ok {
		t.Error("a window with a sliver on screen was restored")
	}
	// Its title bar is below the bottom edge
	if _, ok := Fit(&Rect{100, 1070, 1000, 700}, oneScreen, minimum); ok {
		t.Error("a window below the display was restored")
	}
}

func TestOversizedWindowIsPulledInside(t *testing.T) {
	got, ok := Fit(&Rect{100, 100, 3000, 2000}, oneScreen, minimum)
	if !ok || got != (Rect{0, 0, 1920, 1080}) {
		t.Errorf("Fit = %+v, %v, want the display's size at its corner", got, ok)
	}
	got, _ = Fit(&Rect{1500, 800, 800, 600}, oneScreen, minimum)
	if got.X+got.Width > 1920 || got.Y+got.Height > 1080 {
		t.Errorf("Fit = %+v, want it inside the display", got)
	}
}

func TestSmallWindowIsRaisedToTheMinimum(t *testing.T) {
	got, ok := Fit(&Rect{100, 100, 200, 100}, oneScreen, minimum)
	if !ok || got.Width != 720 || got.Height != 520 {
		t.Errorf("Fit = %+v, %v, want the minimum size", got, ok)
	}
}
