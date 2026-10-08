package main

import (
	"embed"
	"io/fs"
)

// frontendFS holds the embedded frontend: the single-page HTML, its script, and its
// stylesheet. Embedding keeps the whole app a single static binary with no Node
// or npm step
//
//go:embed frontend
var frontendFS embed.FS

// staticFS returns the frontend files rooted at the frontend directory, so they serve
// at the site root
func staticFS() fs.FS {
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		panic(err)
	}
	return sub
}
