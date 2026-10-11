package manifest

import (
	"errors"
	"fmt"
	"strings"
)

// Severity says whether a diagnostic stops a manifest from loading
type Severity uint8

const (
	// Error is a problem that stops the manifest from loading
	Error Severity = iota
	// Warning is something worth an author's attention that does not stop it
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Diagnostic is one problem found in a manifest, with the path to it such as
// layouts.normal.stack[3].part
type Diagnostic struct {
	Severity Severity
	Path     string
	Message  string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Severity.String() + ": " + d.Message
	}
	return d.Severity.String() + ": " + d.Path + ": " + d.Message
}

// Diagnostics is everything found in a manifest, in the order it was found
type Diagnostics []Diagnostic

// HasErrors reports whether any diagnostic stops the manifest from loading
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Err is nil when no diagnostic is an error, and otherwise an error that lists
// each one
func (ds Diagnostics) Err() error {
	var lines []string
	for _, d := range ds {
		if d.Severity == Error {
			lines = append(lines, d.String())
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return errors.New("manifest: " + strings.Join(lines, "\n"))
}

// collector gathers diagnostics as a check walks a manifest
type collector struct{ ds Diagnostics }

func (c *collector) errorf(path, format string, args ...any) {
	c.ds = append(c.ds, Diagnostic{Error, path, fmt.Sprintf(format, args...)})
}

func (c *collector) warnf(path, format string, args ...any) {
	c.ds = append(c.ds, Diagnostic{Warning, path, fmt.Sprintf(format, args...)})
}
