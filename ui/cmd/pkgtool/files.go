package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// renameCmd moves a file, or with --copy copies it, creating the folder it goes
// to. The packaging tools name their output their own way, and the release names
// are fixed, so this gets one to the other the same way on every OS
func renameCmd(args []string) error {
	copyIt := false
	var paths []string
	for _, a := range args {
		if a == "--copy" {
			copyIt = true
		} else {
			paths = append(paths, a)
		}
	}
	if len(paths) != 2 {
		return errors.New("usage: pkgtool rename [--copy] <from> <to>")
	}
	return move(paths[0], paths[1], copyIt)
}

func move(from, to string, copyIt bool) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if !copyIt {
		if err := os.Rename(from, to); err == nil {
			return nil
		}
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if !copyIt {
		return os.Remove(from)
	}
	return nil
}

// rpmArch is the architecture name an rpm file carries
var rpmArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}

// linuxNames moves the AppImage, deb and rpm the Linux packaging wrote into dir
// to out under their release names
func linuxNames(dir, out, version, arch string) error {
	ra, ok := rpmArch[arch]
	if !ok {
		return fmt.Errorf("no rpm architecture for %q", arch)
	}
	for _, f := range []struct{ ext, name string }{
		{".AppImage", fmt.Sprintf("Mimic-%s-linux-%s.AppImage", version, arch)},
		{".deb", fmt.Sprintf("mimic_%s_%s.deb", version, arch)},
		{".rpm", fmt.Sprintf("mimic-%s.%s.rpm", version, ra)},
	} {
		matches, err := filepath.Glob(filepath.Join(dir, "*"+f.ext))
		if err != nil {
			return err
		}
		if len(matches) != 1 {
			return fmt.Errorf("found %d %s files in %s, want one: %s", len(matches), f.ext, dir, strings.Join(matches, ", "))
		}
		if err := move(matches[0], filepath.Join(out, f.name), false); err != nil {
			return err
		}
	}
	return nil
}

func linuxNamesCmd(args []string) error {
	var dir, out, version, arch string
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "-dir":
			dir = args[i+1]
		case "-out":
			out = args[i+1]
		case "-version":
			version = args[i+1]
		case "-arch":
			arch = args[i+1]
		default:
			return fmt.Errorf("unknown flag %s", args[i])
		}
	}
	if dir == "" || out == "" || version == "" || arch == "" {
		return errors.New("usage: pkgtool linux-names -dir d -out o -version v -arch a")
	}
	return linuxNames(dir, out, version, arch)
}
