package resource

import (
	"syscall"
	"unsafe"
)

// memoryStatusEx is the MEMORYSTATUSEX structure GlobalMemoryStatusEx fills in
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var globalMemoryStatusEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// readMemory reads physical memory from GlobalMemoryStatusEx
func readMemory() (total, available uint64) {
	st := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&st))); r == 0 {
		return 0, 0
	}
	return st.TotalPhys, st.AvailPhys
}
