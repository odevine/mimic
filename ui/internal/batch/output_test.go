package batch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Lightning Bolt":         "Lightning Bolt",
		"Fire // Ice":            "Fire - Ice",
		`What?: "A/B" <C>`:       "What-- -A-B- -C-",
		"  ..dots..  ":           "dots",
		"CON":                    "_CON",
		"tab\there":              "tabhere",
		strings.Repeat("a", 300): strings.Repeat("a", 150),
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got, _ := ExpandPath("~/proxies"); got != filepath.Join(home, "proxies") {
		t.Errorf("~ expanded to %q", got)
	}
	if _, err := ExpandPath("proxies"); err == nil {
		t.Error("a relative path should be refused")
	}
}
