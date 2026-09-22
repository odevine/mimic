package template

import (
	"context"
	"testing"

	"github.com/odevine/impasto/raster"
)

type fakeTemplate struct{ name string }

func (f fakeTemplate) Name() string { return f.name }
func (f fakeTemplate) Render(context.Context, RenderRequest) (*raster.Buffer, error) {
	return raster.MustNewBuffer(1, 1), nil
}

func TestRegisterAndGet(t *testing.T) {
	Register("fake-get", "a fake template for tests", func() Template { return fakeTemplate{name: "fake-get"} })

	got, err := Get("fake-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "fake-get" {
		t.Errorf("Name = %q, want fake-get", got.Name())
	}
}

func TestGetUnknown(t *testing.T) {
	if _, err := Get("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown template, got nil")
	}
}

func TestGetReturnsFreshInstance(t *testing.T) {
	// The factory runs per Get, so callers never share one template instance
	Register("fake-fresh", "a fake template for tests", func() Template { return &fakeTemplate{name: "fake-fresh"} })
	a, _ := Get("fake-fresh")
	b, _ := Get("fake-fresh")
	if a == b {
		t.Error("Get returned the same pointer twice; factory should build fresh")
	}
}

func TestNamesSorted(t *testing.T) {
	Register("fake-zeta", "a fake template for tests", func() Template { return fakeTemplate{name: "fake-zeta"} })
	Register("fake-alpha", "a fake template for tests", func() Template { return fakeTemplate{name: "fake-alpha"} })

	names := Names()
	var sawAlpha, sawZeta bool
	var alphaIdx, zetaIdx int
	for i, n := range names {
		switch n {
		case "fake-alpha":
			sawAlpha, alphaIdx = true, i
		case "fake-zeta":
			sawZeta, zetaIdx = true, i
		}
	}
	if !sawAlpha || !sawZeta {
		t.Fatalf("Names missing registered entries: %v", names)
	}
	if alphaIdx > zetaIdx {
		t.Errorf("Names not sorted: alpha at %d after zeta at %d", alphaIdx, zetaIdx)
	}
}

func TestListReportsDescriptions(t *testing.T) {
	Register("fake-described", "what this fake template is for", func() Template { return fakeTemplate{name: "fake-described"} })

	var found *Registration
	for _, r := range List() {
		if r.Name == "fake-described" {
			r := r
			found = &r
			break
		}
	}
	if found == nil {
		t.Fatal("List missing the registered entry")
	}
	if found.Description != "what this fake template is for" {
		t.Errorf("Description = %q, want the registered description", found.Description)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate Register")
		}
	}()
	Register("fake-dup", "a fake template for tests", func() Template { return fakeTemplate{name: "fake-dup"} })
	Register("fake-dup", "a fake template for tests", func() Template { return fakeTemplate{name: "fake-dup"} })
}

func TestRegisterNilFactoryPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on nil factory")
		}
	}()
	Register("fake-nil", "a fake template for tests", nil)
}

func TestRegisterEmptyDescriptionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on empty description")
		}
	}()
	Register("fake-no-description", "", func() Template { return fakeTemplate{name: "fake-no-description"} })
}
