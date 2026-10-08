package mcpstate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteThenRead(t *testing.T) {
	root := t.TempDir()
	want := State{URL: "http://127.0.0.1:1234/mcp", Token: "abc", PID: 42, Home: "/scratch"}
	if err := Write(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root)
	if err != nil || got != want {
		t.Errorf("Read = %+v, %v, want %+v", got, err, want)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(Path(root))
		if st.Mode().Perm() != 0o600 {
			t.Errorf("the state file is %v, want 0600 since it holds the token", st.Mode().Perm())
		}
	}
}

func TestReadWithNothingRunning(t *testing.T) {
	if _, err := Read(t.TempDir()); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Read = %v, want ErrNotRunning", err)
	}
}

func TestRemoveLeavesANewerRunsFile(t *testing.T) {
	root := t.TempDir()
	Write(root, State{PID: 2})
	Remove(root, 1)
	if _, err := Read(root); err != nil {
		t.Errorf("another run's file was removed: %v", err)
	}
	Remove(root, 2)
	if _, err := Read(root); !errors.Is(err, ErrNotRunning) {
		t.Errorf("its own file stayed: %v", err)
	}
}

func TestModuleRootFindsTheUIModule(t *testing.T) {
	root, err := ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "ui" {
		t.Errorf("ModuleRoot = %q, want the ui folder", root)
	}
}
