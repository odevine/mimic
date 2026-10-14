package studio

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// overlayFS is a folder with some of its files replaced by other contents and some
// taken away, so an edit can be checked as the draft would be with it before it is
// written
type overlayFS struct {
	base fs.FS
	over map[string][]byte
	gone []string
}

func (o overlayFS) removed(name string) bool {
	for _, g := range o.gone {
		if g == name {
			return true
		}
	}
	return false
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if o.removed(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	data, ok := o.over[name]
	if !ok {
		return o.base.Open(name)
	}
	return &memFile{Reader: bytes.NewReader(data), name: path.Base(name), size: int64(len(data))}, nil
}

// ReadDir lists a folder with the files put into it, and without the ones taken out
func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(o.base, name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	found := err == nil
	byName := map[string]fs.DirEntry{}
	for _, e := range entries {
		if !o.removed(path.Join(name, e.Name())) {
			byName[e.Name()] = e
		}
	}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	for file, data := range o.over {
		rest, ok := strings.CutPrefix(file, prefix)
		if !ok || rest == "" {
			continue
		}
		found = true
		first, deeper, _ := strings.Cut(rest, "/")
		if deeper != "" {
			if _, there := byName[first]; !there {
				byName[first] = fs.FileInfoToDirEntry(dirInfo(first))
			}
			continue
		}
		byName[first] = fs.FileInfoToDirEntry(&memFile{Reader: bytes.NewReader(data), name: first, size: int64(len(data))})
	}
	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	out := make([]fs.DirEntry, 0, len(byName))
	for _, e := range byName {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

// dirInfo is a folder that exists only because the overlay puts a file inside it
type dirInfo string

func (d dirInfo) Name() string       { return string(d) }
func (d dirInfo) Size() int64        { return 0 }
func (d dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (d dirInfo) ModTime() time.Time { return time.Time{} }
func (d dirInfo) IsDir() bool        { return true }
func (d dirInfo) Sys() any           { return nil }

type memFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Name() string               { return f.name }
func (f *memFile) Size() int64                { return f.size }
func (f *memFile) Mode() fs.FileMode          { return 0o644 }
func (f *memFile) ModTime() time.Time         { return time.Time{} }
func (f *memFile) IsDir() bool                { return false }
func (f *memFile) Sys() any                   { return nil }
