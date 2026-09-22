package main

import (
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// rowText is one result-list line: the card name, its set, and its type
func rowText(d *card.Data) string {
	parts := []string{d.Name}
	if d.SetCode != "" {
		parts = append(parts, strings.ToUpper(d.SetCode))
	}
	if d.TypeLine != "" {
		parts = append(parts, d.TypeLine)
	}
	return strings.Join(parts, "  ·  ")
}

// pngFilename turns a card name into a safe .png filename
func pngFilename(name string) string {
	if name == "" {
		return "card.png"
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
	return cleaned + ".png"
}

// templateDisplay names a template and version for the indicator. An empty
// version is the placeholder fallback
func templateDisplay(name, version string) string {
	switch version {
	case "":
		return name + " (placeholder)"
	case localVersion:
		return name + " · local"
	default:
		return name + " · " + version
	}
}
