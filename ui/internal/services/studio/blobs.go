package studio

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// stashBytes is the most file content the service holds for undo and redo, which
// is a few large images
const stashBytes = 128 << 20

// stash holds the contents of files an edit wrote or removed, by a name taken from
// the contents, so an undo can put them back without the bytes crossing the bridge.
// When it is over its limit the oldest are let go, and an undo that needs one says so
type stash struct {
	mu    sync.Mutex
	files map[string][]byte
	order []string
	size  int
	limit int
}

func newStash(limit int) *stash { return &stash{files: map[string][]byte{}, limit: limit} }

// Put holds a copy of the bytes and returns the name they are held under
func (s *stash) Put(data []byte) string {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:12])
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[name]; ok {
		s.touch(name)
		return name
	}
	s.files[name] = append([]byte(nil), data...)
	s.order = append(s.order, name)
	s.size += len(data)
	for s.size > s.limit && len(s.order) > 1 {
		old := s.order[0]
		s.order = s.order[1:]
		s.size -= len(s.files[old])
		delete(s.files, old)
	}
	return name
}

// Get returns what is held under a name
func (s *stash) Get(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[name]
	if ok {
		s.touch(name)
	}
	return data, ok
}

// touch makes a name the newest
func (s *stash) touch(name string) {
	for i, n := range s.order {
		if n == name {
			s.order = append(append(s.order[:i:i], s.order[i+1:]...), name)
			return
		}
	}
}
