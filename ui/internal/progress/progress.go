// Package progress adapts the progress callbacks of rendering and downloading to
// the step and fraction a job reports, dropping updates too small to show
package progress

import "fmt"

// Render wraps a render progress callback: it drops a repeat step whose fraction
// moved less than 0.01, except the final 1.0, so the event stream stays lean
func Render(emit func(step string, frac float64)) func(step string, frac float64) {
	lastFrac := -1.0
	lastStep := ""
	return func(step string, frac float64) {
		if step == lastStep && frac-lastFrac < 0.01 && frac < 1 {
			return
		}
		lastStep, lastFrac = step, frac
		emit(step, frac)
	}
}

// Bytes adapts a byte-count download callback to a step and fraction emit,
// dropping moves smaller than 0.01 so a large bundle download does not flood the
// stream
func Bytes(emit func(step string, frac float64)) func(done, total int64) {
	last := -1.0
	return func(done, total int64) {
		if total <= 0 {
			return
		}
		frac := float64(done) / float64(total)
		if frac-last < 0.01 && frac < 1 {
			return
		}
		last = frac
		emit(fmt.Sprintf("Downloading %d%%", int(frac*100)), frac)
	}
}
