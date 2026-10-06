// Package resource sizes a batch render to the machine it runs on. A render
// holds a few document-sized buffers, so memory rather than CPU decides how many
// can run at once, and this package turns a resolution and the memory a computer
// has into a worker count
package resource

import "runtime"

const (
	// MaxWorkers is the most renders one run starts at once, however much the
	// machine has
	MaxWorkers = 16

	// renderFixed and bytesPerPixel model what one render adds to the process.
	// They were fitted to measured peaks of the normal template at 300 dpi, 600
	// dpi and its authored 3264 by 4440, where one more worker added about 270
	// MiB, 380 MiB and 1.1 GiB. The pixel term is four 16 byte buffers' worth
	renderFixed   = 200 << 20
	bytesPerPixel = 64

	// baseBytes is what the app takes before its renders, including the fonts and
	// decoders the first one loads. One, two and four native renders measured
	// 1.9, 3.0 and 4.9 GB in all
	baseBytes = 768 << 20

	// fallbackWorkers is how many renders run at once when the computer's memory
	// cannot be read, which is what the app did before it sized runs at all
	fallbackWorkers = 2
)

// Memory is how much physical memory a computer has. A zero field is unknown
type Memory struct {
	Total, Available uint64
}

// ReadMemory reads this computer's memory, with zero fields where the platform
// cannot say
func ReadMemory() Memory {
	total, available := readMemory()
	return Memory{Total: total, Available: available}
}

// Budget is how much memory a run may plan to use. It takes most of what is free
// when the platform reports that, leaving the rest for everything else that is
// open, and half of the total when it does not. Zero means nothing is known
func (m Memory) Budget() uint64 {
	switch {
	case m.Available > 0:
		return m.Available / 10 * 7
	case m.Total > 0:
		return m.Total / 2
	}
	return 0
}

// RenderBytes estimates what one render adds to the process at a width and
// height in pixels
func RenderBytes(width, height int) uint64 {
	if width < 0 || height < 0 {
		return renderFixed
	}
	return renderFixed + uint64(width)*uint64(height)*bytesPerPixel
}

// BaseBytes is the memory a run needs before any render
func BaseBytes() uint64 { return baseBytes }

// RunBytes estimates the memory n renders of perRender bytes each use together
func RunBytes(n int, perRender uint64) uint64 {
	return baseBytes + uint64(max(n, 0))*perRender
}

// Workers is how many renders of perRender bytes each to run at once on a
// computer with this memory and cpus processors. It is the most the memory
// budget holds, no more than the processors, and between one and MaxWorkers
func Workers(m Memory, perRender uint64, cpus int) int {
	n := MaxWorkers
	if cpus > 0 {
		n = min(n, cpus)
	}
	budget := m.Budget()
	if budget == 0 {
		return min(n, fallbackWorkers)
	}
	fit := 1
	if budget > baseBytes && perRender > 0 {
		fit = int((budget - baseBytes) / perRender)
	}
	return max(min(n, fit), 1)
}

// CPUs is the processor count Workers should be given
func CPUs() int { return runtime.NumCPU() }
