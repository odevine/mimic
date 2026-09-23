package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fsListing is one folder as the output picker shows it. Parent is empty at a
// filesystem root
type fsListing struct {
	Path   string   `json:"path"`
	Parent string   `json:"parent"`
	Home   string   `json:"home"`
	Dirs   []string `json:"dirs"`
}

// handleFSList lists the subfolders of path, or of the home folder when path is
// empty, for the output folder picker. The browser cannot hand the server a real
// path, so the picker browses from here. Only folder names are returned, and
// hidden folders are left out
func (s *server) handleFSList(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	p := r.URL.Query().Get("path")
	if strings.TrimSpace(p) == "" {
		p = home
	}
	dir, err := expandPath(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "reading the folder: "+err.Error(), http.StatusNotFound)
		return
	}
	out := fsListing{Path: dir, Home: home, Dirs: []string{}}
	if parent := filepath.Dir(dir); parent != dir {
		out.Parent = parent
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// A symlink to a folder is followed, since a Documents folder is often one
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err == nil && info.IsDir() {
			out.Dirs = append(out.Dirs, e.Name())
		}
	}
	slices.SortFunc(out.Dirs, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	writeJSON(w, out)
}
