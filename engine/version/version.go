// Package version records the engine's own semver so a bundle can declare the
// lowest engine it renders on and the engine can refuse a newer one before a
// render begins.
package version

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Version is the engine's semver, set at release build time with
// -ldflags "-X github.com/odevine/mimic/engine/version.Version=<v>". A dev or
// empty value is treated as newest so local builds render any bundle
var Version = "dev"

// Satisfies reports whether the running engine is new enough to render a bundle
// that requires minEngine. A dev or empty running version is treated as newest
// and always satisfies, so local builds are never blocked. An empty minEngine
// imposes no constraint. A minEngine that is not a valid semver is rejected,
// since compatibility cannot be established
func Satisfies(minEngine string) bool {
	if Version == "dev" || Version == "" {
		return true
	}
	if minEngine == "" {
		return true
	}
	min := canonical(minEngine)
	cur := canonical(Version)
	if !semver.IsValid(min) || !semver.IsValid(cur) {
		return false
	}
	return semver.Compare(cur, min) >= 0
}

// canonical gives a plain X.Y.Z the leading "v" that golang.org/x/mod/semver
// requires. Bundle and tag versions are stored without it
func canonical(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
