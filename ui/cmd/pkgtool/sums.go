package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func sumsCmd(args []string) error {
	fset := flag.NewFlagSet("sums", flag.ExitOnError)
	out := fset.String("out", "SHA256SUMS", "the checksum file to write")
	fset.Parse(args)
	if fset.NArg() == 0 {
		return errNoFiles
	}
	return writeSums(*out, fset.Args())
}

// digest returns the SHA-256 of a file
func digest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// writeSums lists the SHA-256 of every file in sha256sum's format, by file name
// and sorted, so the same files always give the same list
func writeSums(out string, files []string) error {
	var lines []string
	for _, f := range files {
		d, err := digest(f)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(d), filepath.Base(f)))
	}
	sort.Strings(lines)
	return os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
