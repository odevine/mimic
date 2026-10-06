package resource

import "golang.org/x/sys/unix"

// readMemory reads the installed memory. macOS reports no figure for what is
// free that applies the way it does elsewhere, since cached files count as
// reclaimable, so only the total is given and the budget works from half of it
func readMemory() (total, available uint64) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0
	}
	return total, 0
}
