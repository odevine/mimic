package releases

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestCanSwapOnLinuxIsNever(t *testing.T) {
	for _, exe := range []string{"/usr/bin/mimic", "/app/bin/mimic", filepath.Join(t.TempDir(), "mimic")} {
		if CanSwap("linux", exe, env(nil)) {
			t.Errorf("%s was offered a swap", exe)
		}
	}
}

func TestCanSwapOnMacNeedsAWritableBundleFolder(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mimic.app", "Contents", "MacOS", "mimic")
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
	if !CanSwap("windows", filepath.Join(t.TempDir(), "mimic.exe"), env(nil)) {
		t.Error("a per-user install was refused")
	}
	if CanSwap("windows", filepath.Join(t.TempDir(), "missing", "mimic.exe"), env(nil)) {
		t.Error("a folder that is not there was offered a swap")
	}
}
