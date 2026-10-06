package resource

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readMemory reads MemTotal and MemAvailable from /proc/meminfo, which reports
// them in kilobytes
func readMemory() (total, available uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch name {
		case "MemTotal":
			total = kb << 10
		case "MemAvailable":
			available = kb << 10
		}
	}
	return total, available
}
