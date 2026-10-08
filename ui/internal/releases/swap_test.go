package releases

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestCanSwapOnLinuxNeedsAnAppImage(t *testing.T) {
	dir := t.TempDir()
	if CanSwap("linux", "/usr/bin/mimic", env(nil)) {
		t.Error("a package install was offered a swap")
	}
	if !CanSwap("linux", "/tmp/.mount_x/usr/bin/mimic", env(map[string]string{"APPIMAGE": filepath.Join(dir, "Mimic.AppImage")})) {
		t.Error("an AppImage in a folder the user owns was refused")
	}
}

func TestCanSwapOnMacNeedsAWritableBundleFolder(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Mimic.app", "Contents", "MacOS", "mimic")
	if !CanSwap("darwin", exe, env(nil)) {
		t.Error("a bundle in a writable folder was refused")
	}
	if CanSwap("darwin", "/tmp/go-build/mimic", env(nil)) {
		t.Error("a bare binary was treated as a bundle")
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0o700)
		if CanSwap("darwin", exe, env(nil)) {
			t.Error("a bundle in a read-only folder was offered a swap")
		}
	}
}

func TestCanSwapOnWindowsNeedsAWritableFolder(t *testing.T) {
	if !CanSwap("windows", filepath.Join(t.TempDir(), "Mimic.exe"), env(nil)) {
		t.Error("a per-user install was refused")
	}
	if CanSwap("windows", filepath.Join(t.TempDir(), "missing", "Mimic.exe"), env(nil)) {
		t.Error("a folder that is not there was offered a swap")
	}
}
