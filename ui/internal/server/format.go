package server

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
