//go:build !linux && !darwin && !windows

package resource

// readMemory reports nothing on a platform this package has no reader for, so
// runs fall back to a fixed worker count
func readMemory() (total, available uint64) { return 0, 0 }
