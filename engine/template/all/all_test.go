package all

import (
	"testing"

	"github.com/odevine/mimic/engine/template"
)

// TestEveryTemplateIsWellRegistered checks every template this package
// blank-imports against the registry's own contract: a nonempty description,
// and a Get that returns something reporting its own name back. One test here
// covers every template, rather than each template's package repeating it
func TestEveryTemplateIsWellRegistered(t *testing.T) {
	list := template.List()
	if len(list) == 0 {
		t.Fatal("no templates registered; is this package's blank import list stale?")
	}
	for _, r := range list {
		t.Run(r.Name, func(t *testing.T) {
			if r.Description == "" {
				t.Error("registered with an empty description")
			}
			tmpl, err := template.Get(r.Name)
			if err != nil {
				t.Fatalf("Get(%q): %v", r.Name, err)
			}
			if got := tmpl.Name(); got != r.Name {
				t.Errorf("Name() = %q, want %q", got, r.Name)
			}
		})
	}
}
