package studio

import (
	"bytes"
	"io/fs"
	"path"
	"time"
)

// overlayFS is a folder with some of its files replaced by other contents, so an
// edit can be checked as the draft would be with it before it is written
type overlayFS struct {
	base fs.FS
	over map[string][]byte
}

func (o overlayFS) Open(name string) (fs.File, error) {
	data, ok := o.over[name]
	if !ok {
		return o.base.Open(name)
	}
	return &memFile{Reader: bytes.NewReader(data), name: path.Base(name), size: int64(len(data))}, nil
}

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
