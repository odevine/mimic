package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// numeric is the dotted number a version resource takes, without any prerelease
// or build suffix
func numeric(version string) string {
	v := strings.TrimPrefix(version, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func stampCmd(args []string) error {
	fs := flag.NewFlagSet("stamp", flag.ExitOnError)
	version := fs.String("version", "", "the release version, without the ui/v prefix")
	dir := fs.String("dir", "build", "the build assets folder")
	fs.Parse(args)
	return stamp(*dir, *version)
}

// stamp writes version into the build assets that carry one: the macOS
// Info.plist, the Windows version resource and the identity file. The installer
// and the packages read the version from those or from the environment, so this
// is the one place a release's version lands in them. The files are edited in
// the build checkout and never committed
func stamp(dir, version string) error {
	n := numeric(version)
	if !versionPattern.MatchString(n) {
		return fmt.Errorf("%q is not a version like 1.2.3", version)
	}
	edits := []struct {
		file string
		re   *regexp.Regexp
		to   string
	}{
		{"darwin/Info.plist", regexp.MustCompile(`(<key>CFBundleVersion</key>\s*<string>)[^<]*`), "${1}" + n},
		{"darwin/Info.plist", regexp.MustCompile(`(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*`), "${1}" + n},
		{"windows/info.json", regexp.MustCompile(`("file_version":\s*")[^"]*`), "${1}" + n},
		{"windows/info.json", regexp.MustCompile(`("ProductVersion":\s*")[^"]*`), "${1}" + n},
		{"config.yml", regexp.MustCompile(`(?m)^(  version: ")[^"]*`), "${1}" + n},
	}
	for _, e := range edits {
		path := filepath.Join(dir, e.file)
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !e.re.Match(raw) {
			return fmt.Errorf("%s has no version field to stamp", path)
		}
		if err := os.WriteFile(path, e.re.ReplaceAll(raw, []byte(e.to)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

var errNoFiles = errors.New("no files given")
