// Package testassets holds the inputs and expected outputs the tests share.
// Scryfall holds card objects shaped like Scryfall's own, which the fake
// upstream searches
package testassets

import "embed"

// Scryfall is a folder of card objects, one JSON file per printing
//
//go:embed scryfall/*.json
var Scryfall embed.FS
