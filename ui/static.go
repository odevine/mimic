package main

import (
	"embed"
	"io/fs"
)

// webFS holds the embedded frontend: the single-page HTML, its script, and its
// stylesheet. Embedding keeps the whole app a single static binary with no Node
// or npm step
//
//go:embed web
var webFS embed.FS

// staticFS returns the frontend files rooted at the web directory, so they serve
// at the site root
func staticFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
