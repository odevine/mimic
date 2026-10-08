// Package mcpstate is the file a running mcp test build leaves behind so a
// command in another process can find its endpoint. It lives under the ui
// module's bin folder, which git ignores
package mcpstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// State is what the running app published
type State struct {
	// URL is the MCP endpoint, such as http://127.0.0.1:54321/mcp
	URL string `json:"url"`
	// Token is the bearer token the endpoint requires
	Token string `json:"token"`
	// PID is the process to signal to stop the app
	PID int `json:"pid"`
	// Home is the scratch folder the app runs in
	Home string `json:"home"`
}

// ErrNotRunning means no state file exists
var ErrNotRunning = errors.New("no mcp test build is running: start one with the dev:mcp task")

// ModuleRoot finds the folder holding the ui module's go.mod, looking upward
// from the working directory
func ModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if raw, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil &&
			(bytes.HasPrefix(raw, []byte("module github.com/odevine/mimic/ui\n")) || bytes.Contains(raw, []byte("\nmodule github.com/odevine/mimic/ui\n"))) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run this from inside the ui module")
		}
		dir = parent
	}
}

// Path is where the state file lives for the module at root
func Path(root string) string { return filepath.Join(root, "bin", "mcp.json") }

// Write publishes the state, replacing any earlier one
func Write(root string, s State) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	p := Path(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// The token lets any local process drive the app, so the file is private
	return os.WriteFile(p, raw, 0o600)
}

// Read returns the published state, or ErrNotRunning
func Read(root string) (State, error) {
	raw, err := os.ReadFile(Path(root))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, ErrNotRunning
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// Remove deletes the state file, if it still describes pid. A newer run's file
// is left alone
func Remove(root string, pid int) {
	if s, err := Read(root); err == nil && s.PID == pid {
		os.Remove(Path(root))
	}
}
