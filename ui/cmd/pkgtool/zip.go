package main

import (
	"archive/zip"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func zipCmd(args []string) error {
	fset := flag.NewFlagSet("zip", flag.ExitOnError)
	out := fset.String("out", "", "the zip file to write")
	fset.Parse(args)
	if *out == "" || fset.NArg() != 1 {
		return errors.New("usage: pkgtool zip -out file.zip <file or folder>")
	}
	return zipOne(*out, fset.Arg(0))
}

// zipOne writes a zip holding exactly one top-level entry, the file or folder
// named, which is the shape the updater unpacks. Permissions are kept, since an
// app bundle's binary has to stay executable
func zipOne(out, src string) error {
	src = filepath.Clean(src)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	base := filepath.Dir(src)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			h.Name += "/"
			_, err = zw.CreateHeader(h)
			return err
		}
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}
