// Package drafts keeps the templates a person is building in the editor. Each
// draft is a template's source folder, the files an author edits, under one folder
// of the app's config. A draft is never installed until it is asked to be, so
// editing one cannot change what an installed template renders
package drafts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/odevine/mimic/engine/author"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
)

const (
	// Owner is the owner half of a draft's template id, so a draft named "My frame"
	// is "user.my-frame"
	Owner = "user"

	metaFile    = ".studio.json"
	sourceFile  = "template.json"
	layoutsDir  = "layouts"
	maxNameLen  = 80
	maxSlugLen  = 40
	stagingMark = ".tmp-"
)

// idRe is the form of a draft's id, which is the name of its folder
var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Store is the folder that holds the drafts
type Store struct{ dir string }

// NewStore returns the store over dir, which is created when the first draft is
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Summary is one draft as the editor lists it. ID names its folder, and
// TemplateID is the id its template carries. Problem is set for a draft whose
// files cannot be read, which a person can still delete
type Summary struct {
	ID         string    `json:"id"`
	TemplateID string    `json:"templateId"`
	Name       string    `json:"name"`
	Updated    time.Time `json:"updated"`
	// ForkedFrom names the template and version the draft began as a copy of,
	// and is empty for one that began blank
	ForkedFrom string `json:"forkedFrom,omitempty"`
	Problem    string `json:"problem,omitempty"`
}

// Meta is what the editor remembers about a draft beside its template, which is
// not part of the template itself
type Meta struct {
	ForkedFrom string `json:"forkedFrom,omitempty"`
	// TestCard is the card the draft renders while it is edited, and Face is which
	// of its images
	TestCard *card.Data `json:"testCard,omitempty"`
	Face     int        `json:"face,omitempty"`
}

// Slug turns a name into the folder name a draft with that name would take
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	s := b.String()
	if len(s) > maxSlugLen {
		s = strings.TrimRight(s[:maxSlugLen], "-")
	}
	if s == "" {
		return "template"
	}
	return s
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", apierr.New(apierr.BadRequest, "Give the template a name")
	case len([]rune(name)) > maxNameLen:
		return "", apierr.Newf(apierr.BadRequest, "A name is at most %d characters", maxNameLen)
	}
	return name, nil
}

// path is the folder of a draft, and refuses an id that is not one so a caller
// cannot reach outside the store
func (s *Store) path(id string) (string, error) {
	if !idRe.MatchString(id) {
		return "", apierr.New(apierr.BadRequest, "That is not a draft")
	}
	return filepath.Join(s.dir, id), nil
}

// FS is the draft's folder, for the engine to read
func (s *Store) FS(id string) (fs.FS, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		return nil, apierr.New(apierr.NotFound, "That draft no longer exists")
	}
	return os.DirFS(p), nil
}

// TemplateID is the id a draft's template carries
func TemplateID(id string) string { return Owner + "." + id }

// List returns the drafts, newest first. A draft whose files cannot be read is
// listed with the reason
func (s *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Summary{}
	for _, e := range entries {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		out = append(out, s.summary(e.Name()))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get is the summary of one draft
func (s *Store) Get(id string) (Summary, error) {
	p, err := s.path(id)
	if err != nil {
		return Summary{}, err
	}
	if _, err := os.Stat(p); err != nil {
		return Summary{}, apierr.New(apierr.NotFound, "That draft no longer exists")
	}
	return s.summary(id), nil
}

func (s *Store) summary(id string) Summary {
	dir := filepath.Join(s.dir, id)
	sum := Summary{ID: id, TemplateID: TemplateID(id), Name: id, Updated: newest(dir)}
	raw, err := os.ReadFile(filepath.Join(dir, sourceFile))
	if err != nil {
		sum.Problem = "Its template.json cannot be read"
		return sum
	}
	var src struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &src); err != nil {
		sum.Problem = "Its template.json is not valid JSON"
		return sum
	}
	if src.Name != "" {
		sum.Name = src.Name
	}
	if src.ID != "" {
		sum.TemplateID = src.ID
	}
	if m, err := s.Meta(id); err == nil {
		sum.ForkedFrom = m.ForkedFrom
	}
	return sum
}

// newest is the latest modification time among the files an edit changes
func newest(dir string) time.Time {
	var latest time.Time
	note := func(p string) {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	note(filepath.Join(dir, sourceFile))
	note(filepath.Join(dir, metaFile))
	if entries, err := os.ReadDir(filepath.Join(dir, layoutsDir)); err == nil {
		for _, e := range entries {
			note(filepath.Join(dir, layoutsDir, e.Name()))
		}
	}
	return latest
}

// unique is the folder name for a new draft: the name's slug, with a number added
// when another draft has it
func (s *Store) unique(name string) string {
	base := Slug(name)
	slug := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(s.dir, slug)); errors.Is(err, fs.ErrNotExist) {
			return slug
		}
		suffix := fmt.Sprintf("-%d", n)
		slug = strings.TrimRight(base[:min(len(base), maxSlugLen-len(suffix))], "-") + suffix
	}
}

// Create makes a draft from the engine's starter, which renders a card as soon as
// it is opened. dpi sets the canvas, and zero is the starter's default
func (s *Store) Create(name string, dpi int) (Summary, error) {
	name, err := cleanName(name)
	if err != nil {
		return Summary{}, err
	}
	slug := s.unique(name)
	files, err := author.Starter(author.StarterOptions{ID: TemplateID(slug), Name: name, DPI: dpi})
	if err != nil {
		return Summary{}, apierr.Wrap(apierr.BadRequest, "Could not make the template", err)
	}
	return s.write(slug, files, Meta{})
}

// Fork makes a draft from a copy of an installed template. The copy keeps the
// original's license and credits and adds a credit that names the original and its
// version, which is the only record of where it came from. It is independent from
// then on
func (s *Store) Fork(p template.AssetProvider, header template.BundleHeader, name string) (Summary, error) {
	name, err := cleanName(name)
	if err != nil {
		return Summary{}, err
	}
	files, _, err := author.Extract(p, author.ExtractOptions{Version: "0.1.0", MinEngine: header.MinEngine})
	if err != nil {
		return Summary{}, apierr.Wrap(apierr.BadRequest, "Could not copy the template", err)
	}
	slug := s.unique(name)

	var src author.Source
	if err := json.Unmarshal(files[sourceFile], &src); err != nil {
		return Summary{}, err
	}
	was := src.ID
	if header.Version != "" {
		was += " " + header.Version
	}
	credit := manifest.Credit{Name: src.Name, URL: src.Author.URL, Note: "Forked from " + was}
	if src.Author.Name != "" {
		credit.Name = src.Author.Name
	}
	src.Credits = append(src.Credits, credit)
	src.ID, src.Name, src.Author = TemplateID(slug), name, manifest.Author{}
	out, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return Summary{}, err
	}
	files[sourceFile] = append(out, '\n')
	return s.write(slug, files, Meta{ForkedFrom: was})
}

// write puts the files in a folder of their own, in one step so a draft is never
// seen half written
func (s *Store) write(slug string, files map[string][]byte, meta Meta) (Summary, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Summary{}, err
	}
	staging, err := os.MkdirTemp(s.dir, stagingMark)
	if err != nil {
		return Summary{}, err
	}
	fail := func(err error) (Summary, error) {
		os.RemoveAll(staging)
		return Summary{}, err
	}
	for name, data := range files {
		if clean := path.Clean(name); clean != name || !fs.ValidPath(name) {
			return fail(fmt.Errorf("drafts: %q is not a file inside the draft", name))
		}
		dst := filepath.Join(staging, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fail(err)
		}
	}
	if meta != (Meta{}) {
		if err := writeMeta(staging, meta); err != nil {
			return fail(err)
		}
	}
	if err := os.Rename(staging, filepath.Join(s.dir, slug)); err != nil {
		return fail(err)
	}
	return s.summary(slug), nil
}

// Rename changes the name a draft shows. Its id and its folder stay, so anything
// that refers to it keeps working
func (s *Store) Rename(id, name string) (Summary, error) {
	name, err := cleanName(name)
	if err != nil {
		return Summary{}, err
	}
	p, err := s.path(id)
	if err != nil {
		return Summary{}, err
	}
	file := filepath.Join(p, sourceFile)
	raw, err := os.ReadFile(file)
	if err != nil {
		return Summary{}, apierr.New(apierr.NotFound, "That draft no longer exists")
	}
	var src author.Source
	if err := json.Unmarshal(raw, &src); err != nil {
		return Summary{}, apierr.Wrap(apierr.BadRequest, "Its template.json is not valid JSON", err)
	}
	src.Name = name
	out, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return Summary{}, err
	}
	if err := writeAtomic(file, append(out, '\n')); err != nil {
		return Summary{}, err
	}
	return s.summary(id), nil
}

// Delete removes a draft and everything in it
func (s *Store) Delete(id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return apierr.New(apierr.NotFound, "That draft no longer exists")
	}
	return os.RemoveAll(p)
}

// WriteFile replaces a file of a draft in one step. The name is a slash separated
// path inside the draft, so one that leaves it is refused, and so is one that does
// not name a file already there, which keeps an edit from adding files the draft
// does not have
func (s *Store) WriteFile(id, name string, data []byte) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if !fs.ValidPath(name) || name == "." {
		return apierr.Newf(apierr.BadRequest, "%q is not a file in the template", name)
	}
	file := filepath.Join(p, filepath.FromSlash(name))
	st, err := os.Stat(file)
	if err != nil || !st.Mode().IsRegular() {
		return apierr.Newf(apierr.NotFound, "The template has no file %s", name)
	}
	return writeAtomic(file, data)
}

// Meta reads what the editor remembers about a draft. A draft with nothing
// remembered has the zero Meta
func (s *Store) Meta(id string) (Meta, error) {
	p, err := s.path(id)
	if err != nil {
		return Meta{}, err
	}
	raw, err := os.ReadFile(filepath.Join(p, metaFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Meta{}, nil
	}
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		// What the editor remembers is not worth failing a draft over
		return Meta{}, nil
	}
	return m, nil
}

// SaveMeta keeps what the editor remembers about a draft
func (s *Store) SaveMeta(id string, m Meta) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return apierr.New(apierr.NotFound, "That draft no longer exists")
	}
	return writeMeta(p, m)
}

func writeMeta(dir string, m Meta) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, metaFile), append(raw, '\n'))
}

// writeAtomic replaces a file in one step, so a crash leaves the old one or the
// new one and never half of either
func writeAtomic(file string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), stagingMark+"*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
