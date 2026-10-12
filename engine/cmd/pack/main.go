// Command pack builds a .mimic bundle from a template's source folder
//
//	pack [-version 1.2.3] [-o out.mimic] <folder>
//
// The folder holds template.json and a layouts folder, and the bundle is written to
// <id>-<version>.mimic beside the folder unless -o names it. The version in
// template.json is used unless -version replaces it
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/odevine/mimic/engine/author"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pack:", err)
		os.Exit(1)
	}
}

func run() error {
	// The folder may come before or after the flags
	args := os.Args[1:]
	var dir string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		dir, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	version := fs.String("version", "", "replaces the version in template.json")
	out := fs.String("o", "", "the bundle to write, by default <id>-<version>.mimic beside the folder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if dir == "" && fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	if dir == "" || fs.NArg() > 1 || dir != "" && fs.NArg() == 1 && fs.Arg(0) != dir {
		return fmt.Errorf("usage: pack [-version v] [-o out.mimic] <folder>")
	}

	built, ds := author.Build(os.DirFS(dir), author.Options{Version: *version})
	for _, d := range ds {
		fmt.Fprintln(os.Stderr, d)
	}
	if built == nil {
		return fmt.Errorf("the template has errors")
	}
	path := *out
	if path == "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		path = filepath.Join(filepath.Dir(abs), fmt.Sprintf("%s-%s.mimic", built.Header.ID, built.Header.Version))
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if err := built.WriteZip(w); err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d images)\n", path, len(built.Assets))
	return nil
}
