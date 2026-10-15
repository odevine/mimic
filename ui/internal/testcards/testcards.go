// Package testcards keeps the cards a person saved to test templates with. Each is a
// card as it was when saved, with any edits to its fields in it, so choosing one needs
// no lookup and works offline
package testcards

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/drafts"
)

const (
	// Max is how many cards can be kept, so the file stays small
	Max = 24
	// MaxLabel is the longest label a card can be given, in characters
	MaxLabel = 40
	idPrefix = "user-"
)

// Saved is a card kept as a test card. Face is the one of its images that was shown
type Saved struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Card  card.Data `json:"card"`
	Face  int       `json:"face,omitempty"`
}

// Store is the file the cards are kept in, which is made when the first one is saved
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns the store over the file at path
func NewStore(path string) *Store { return &Store{path: path} }

// IsID reports whether an id is one of a saved card, which no built-in test card has
func IsID(id string) bool { return strings.HasPrefix(id, idPrefix) }

type file struct {
	Cards []Saved `json:"cards"`
}

func (s *Store) read() ([]Saved, error) {
	if s.path == "" {
		return nil, apierr.New(apierr.Internal, "There is nowhere to keep test cards")
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, apierr.Wrap(apierr.Internal, "Could not read your saved test cards", err)
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, apierr.Wrap(apierr.Internal, "Your saved test cards are not readable", err)
	}
	return f.Cards, nil
}

func (s *Store) write(list []Saved) error {
	raw, err := json.MarshalIndent(file{Cards: list}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return apierr.Wrap(apierr.Internal, "Could not keep the test card", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".testcards-*")
	if err != nil {
		return apierr.Wrap(apierr.Internal, "Could not keep the test card", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return apierr.Wrap(apierr.Internal, "Could not keep the test card", err)
	}
	if err := tmp.Close(); err != nil {
		return apierr.Wrap(apierr.Internal, "Could not keep the test card", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return apierr.Wrap(apierr.Internal, "Could not keep the test card", err)
	}
	return nil
}

// List returns the saved cards in the order they were saved
func (s *Store) List() ([]Saved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

// Get returns the saved card with an id
func (s *Store) Get(id string) (Saved, error) {
	list, err := s.List()
	if err != nil {
		return Saved{}, err
	}
	for _, c := range list {
		if c.ID == id {
			return c, nil
		}
	}
	return Saved{}, apierr.Newf(apierr.NotFound, "%q is not one of your saved test cards", id)
}

// Add keeps a card under a label and returns it with its id. A label another card has
// is refused, since the two could not be told apart in the list
func (s *Store) Add(label string, c card.Data, face int) (Saved, error) {
	label = strings.Join(strings.Fields(label), " ")
	switch {
	case label == "":
		return Saved{}, apierr.New(apierr.BadRequest, "Give the test card a name")
	case utf8.RuneCountInString(label) > MaxLabel:
		return Saved{}, apierr.Newf(apierr.BadRequest, "A test card's name can be at most %d characters", MaxLabel)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return Saved{}, err
	}
	if len(list) >= Max {
		return Saved{}, apierr.Newf(apierr.Conflict, "You can keep %d test cards. Remove one to save another", Max)
	}
	taken := map[string]bool{}
	for _, e := range list {
		if strings.EqualFold(e.Label, label) {
			return Saved{}, apierr.Newf(apierr.Conflict, "A test card is already called %q", e.Label)
		}
		taken[e.ID] = true
	}
	base := idPrefix + drafts.Slug(label)
	id := base
	for n := 2; taken[id]; n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	saved := Saved{ID: id, Label: label, Card: c, Face: face}
	return saved, s.write(append(list, saved))
}

// Remove forgets a saved card
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	for i, c := range list {
		if c.ID == id {
			return s.write(append(list[:i:i], list[i+1:]...))
		}
	}
	return apierr.Newf(apierr.NotFound, "%q is not one of your saved test cards", id)
}
