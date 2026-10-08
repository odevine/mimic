package releases

import (
	"os"
	"path/filepath"
	"strings"
)

// CanSwap reports whether an app running from exe can replace itself with a
// downloaded release. The update swaps the app in place, so the app has to sit
// where the user can write. A Linux package install belongs to the system package
// manager and is out of reach, while an AppImage names itself in the APPIMAGE
// environment variable and is a single file the user owns
func CanSwap(goos, exe string, getenv func(string) string) bool {
	switch goos {
	case "linux":
		return getenv("APPIMAGE") != "" && writable(filepath.Dir(getenv("APPIMAGE")))
	case "darwin":
		// The bundle is Mimic.app/Contents/MacOS/mimic, and it is the bundle
		// that gets replaced, inside whatever folder holds it
		if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
			return writable(filepath.Dir(exe[:i+len(".app")]))
		}
		return false
	default:
		return writable(filepath.Dir(exe))
	}
}

// writable reports whether files can be created in dir
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".mimic-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
