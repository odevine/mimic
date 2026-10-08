// Package buildinfo holds the version a release build stamps into the binary
package buildinfo

import "github.com/odevine/mimic/engine/version"

// Version is the ui release, set at build time with
// -ldflags "-X github.com/odevine/mimic/ui/internal/buildinfo.Version=<v>". A
// build that was not stamped reports dev
var Version = "dev"

// Commit is the git commit of the build, stamped the same way, and empty when
// it was not
var Commit = ""

// Engine is the engine release this build renders with, stamped through the
// engine module's own variable, and is dev when that was stamped empty
func Engine() string {
	if version.Version == "" {
		return "dev"
	}
	return version.Version
}
