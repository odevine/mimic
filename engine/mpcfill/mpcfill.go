// Package mpcfill lays rendered cards out as an MPC Autofill project: a
// directory of PNGs plus the cards.xml order file that the MPC Autofill desktop
// tool uploads to MakePlayingCards, and that the MPC Autofill website imports
// once the same directory is added as a local folder source.
package mpcfill

import (
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Stock is a MakePlayingCards cardstock, spelled exactly as MPC Autofill expects
type Stock string

// The cardstocks MPC Autofill supports. Only P10 has no foil option
const (
	S27 Stock = "(S27) Smooth"
	S30 Stock = "(S30) Standard Smooth"
	S33 Stock = "(S33) Superior Smooth"
	M31 Stock = "(M31) Linen"
	P10 Stock = "(P10) Plastic"
)

// MaxProjectSize is the most cards one MPC Autofill project holds
const MaxProjectSize = 612

// The largest image the website lists with its default search filters. An
// image it hides is swapped for another on import
const (
	MaxImageBytes = 30_000_000
	MaxDPI        = 1500
)

// DPI is the resolution MPC Autofill gives an image of the given pixel height
func DPI(height int) int {
	return 10 * int(math.Round(float64(height)*300/1110/10))
}

// OrderFile is the order file's name
const OrderFile = "cards.xml"

// Subfolders of a project directory. MPC Autofill types an image by its parent
// folder's name, so the cardback needs its own folder and fronts cannot sit in
// the root
const (
	FrontsDir   = "fronts"
	BacksDir    = "backs"
	CardbackDir = "cardback"
)

var (
	// ErrNoCards reports a project with an empty card list
	ErrNoCards = errors.New("mpcfill: project has no cards")
	// ErrTooManyCards reports a project over MaxProjectSize
	ErrTooManyCards = errors.New("mpcfill: project exceeds the maximum size")
	// ErrStock reports a cardstock that is not one of the Stock constants
	ErrStock = errors.New("mpcfill: unknown cardstock")
	// ErrFoil reports foil on a cardstock that has no foil option
	ErrFoil = errors.New("mpcfill: cardstock does not support foil")
	// ErrNoCardback reports single-faced cards in a project without a cardback
	ErrNoCardback = errors.New("mpcfill: single-faced cards need a cardback")
	// ErrMissingImage reports a planned image that is not on disk
	ErrMissingImage = errors.New("mpcfill: image not written")
)

// Face is one side of a card. Names are normalized into file names, see Plan
type Face struct {
	Name string
}

// Card is one entry in a project. A nil Back gives the card the project's
// overall cardback, a non-nil Back makes it double-faced. Quantity is how
// many copies to print, any number under 1 is 1
type Card struct {
	Front    Face
	Back     *Face
	Quantity int
}

// Project is a complete order. An empty Stock is S30, the default in both
// MPC Autofill tools. Cardback is shared by every card without its own Back,
// and may be nil only when every card is double-faced (otherwise is required)
type Project struct {
	Stock    Stock
	Foil     bool
	Cardback *Face
	Cards    []Card
}

// Layout is a validated Project with a file for every face. Paths are
// slash-separated and relative to the project directory. Write each face's PNG
// to its path, then call WriteOrder
type Layout struct {
	// Cards holds the files of Project.Cards, in the same order
	Cards []CardFiles
	// Cardback is the shared cardback's file, empty when there is none
	Cardback string

	order *layout
}

// CardFiles is where one card's faces go. Back is empty for a card that uses
// the project's cardback
type CardFiles struct {
	Front string
	Back  string
}

// Plan validates p and gives every face a file under fronts/, backs/, or
// cardback/. Face names keep only ASCII letters, digits, hyphens, and
// apostrophes, so "Fire // Ice" becomes "Fire Ice.png". See the README for why
func Plan(p Project) (*Layout, error) {
	l, err := p.layout()
	if err != nil {
		return nil, err
	}
	out := &Layout{Cards: make([]CardFiles, len(p.Cards)), order: l}
	for i, f := range l.fronts {
		out.Cards[i].Front = f.path
	}
	for _, b := range l.backs {
		out.Cards[b.card].Back = b.path
	}
	if l.cardback != nil {
		out.Cardback = l.cardback.path
	}
	return out, nil
}

// WriteOrder writes OrderFile into dir once every image of l is there. The
// desktop tool skips an empty file as missing, so one counts as not written
func WriteOrder(dir string, l *Layout) error {
	for _, e := range l.order.entries() {
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(e.path)))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
			return fmt.Errorf("%w: %s", ErrMissingImage, e.path)
		}
	}
	out, err := l.order.marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, OrderFile), out, 0o644); err != nil {
		return fmt.Errorf("mpcfill: write %s: %w", OrderFile, err)
	}
	return nil
}

// entry is one image file and the slots it fills on its face
type entry struct {
	card  int    // index into Project.Cards, unused for the cardback
	path  string // relative to the project directory, slash-separated
	query string
	slots []int
}

// layout is a validated project resolved to files and slots
type layout struct {
	stock    Stock
	foil     bool
	quantity int
	fronts   []entry
	backs    []entry
	cardback *entry
}

func (p Project) layout() (*layout, error) {
	stock := p.Stock
	if stock == "" {
		stock = S30
	}
	switch stock {
	case S27, S30, S33, M31:
	case P10:
		if p.Foil {
			return nil, fmt.Errorf("%w: %s", ErrFoil, stock)
		}
	default:
		return nil, fmt.Errorf("%w: %q", ErrStock, stock)
	}
	if len(p.Cards) == 0 {
		return nil, ErrNoCards
	}

	l := &layout{stock: stock, foil: p.Foil}
	fronts, backs := names{}, names{}
	for i, c := range p.Cards {
		if c.Back == nil && p.Cardback == nil {
			return nil, fmt.Errorf("%w: card %d", ErrNoCardback, i)
		}

		copies := max(c.Quantity, 1)
		if copies > MaxProjectSize-l.quantity {
			return nil, fmt.Errorf("%w: more than %d cards", ErrTooManyCards, MaxProjectSize)
		}
		slots := make([]int, copies)
		for j := range slots {
			slots[j] = l.quantity + j
		}
		l.quantity += copies

		l.fronts = append(l.fronts, fronts.entry(FrontsDir, c.Front, i, slots))
		if c.Back != nil {
			l.backs = append(l.backs, backs.entry(BacksDir, *c.Back, i, slots))
		}
	}

	if p.Cardback != nil {
		cb := names{}.entry(CardbackDir, *p.Cardback, 0, nil)
		l.cardback = &cb
	}
	return l, nil
}

func (l *layout) entries() []entry {
	all := append(append([]entry{}, l.fronts...), l.backs...)
	if l.cardback != nil {
		all = append(all, *l.cardback)
	}
	return all
}

// names hands out file stems unique within one folder. Keys are lowercased
// because the common desktop filesystems are case-insensitive
type names map[string]bool

func (n names) entry(dir string, f Face, card int, slots []int) entry {
	base := stem(f.Name)
	s := base
	for i := 2; n[strings.ToLower(s)] || windowsReserved(s); i++ {
		s = base + " " + strconv.Itoa(i)
	}
	n[strings.ToLower(s)] = true
	return entry{
		card:  card,
		path:  path.Join(dir, s+".png"),
		query: strings.ToLower(s),
		slots: slots,
	}
}

// windowsReserved reports device names Windows refuses as file names, whatever
// the extension (I love Windows)
func windowsReserved(stem string) bool {
	s := strings.ToLower(stem)
	switch s {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(s) == 4 && (s[:3] == "com" || s[:3] == "lpt") && '1' <= s[3] && s[3] <= '9'
}

// maxStem keeps names well inside the 255-byte file name limit once a suffix
// and extension are added. It's paranoid, but safe.
const maxStem = 100

// stem keeps only characters the website's search neither strips nor reads as
// a set code, so the lowercased stem always finds its own file. It is ASCII-only
// because a non-ASCII name can come back from the filesystem in another Unicode
// normalization (filesystems are great!)
func stem(name string) string {
	var b strings.Builder
	letter, gap := false, false
	for _, r := range asciiFold.Replace(name) {
		isLetter := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
		if !isLetter && !('0' <= r && r <= '9') && r != '-' && r != '\'' {
			gap = b.Len() > 0
			continue
		}
		sep := ""
		if gap {
			sep = " "
		}
		if b.Len()+len(sep)+1 > maxStem {
			break
		}
		b.WriteString(sep)
		b.WriteRune(r)
		gap = false
		letter = letter || isLetter
	}
	if !letter {
		return "Card"
	}
	return b.String()
}

// asciiFold maps the accented letters of Latin-1, and typographic apostrophes,
// to plain ASCII
var asciiFold = func() *strings.Replacer {
	const accented = "ÀÁÂÃÄÅàáâãäåÇçÈÉÊËèéêëÌÍÎÏìíîïÑñÒÓÔÕÖØòóôõöøÙÚÛÜùúûüÝýÿ"
	const plain = "AAAAAAaaaaaaCcEEEEeeeeIIIIiiiiNnOOOOOOooooooUUUUuuuuYyy"
	pairs := []string{"Æ", "AE", "æ", "ae", "Œ", "OE", "œ", "oe", "ß", "ss", "’", "'", "‘", "'"}
	for i, r := range []rune(accented) {
		pairs = append(pairs, string(r), plain[i:i+1])
	}
	return strings.NewReplacer(pairs...)
}()

// The order file schema, in the element order the website writes
type (
	xmlOrder struct {
		XMLName  xml.Name   `xml:"order"`
		Details  xmlDetails `xml:"details"`
		Fronts   xmlFace    `xml:"fronts"`
		Backs    *xmlFace   `xml:"backs,omitempty"`
		Cardback string     `xml:"cardback,omitempty"`
	}
	xmlFace struct {
		Cards []xmlCard `xml:"card"`
	}
	xmlDetails struct {
		Quantity int    `xml:"quantity"`
		Stock    string `xml:"stock"`
		Foil     bool   `xml:"foil"`
	}
	xmlCard struct {
		ID         string `xml:"id"`
		SourceType string `xml:"sourceType"`
		Slots      string `xml:"slots"`
		Name       string `xml:"name"`
		Query      string `xml:"query"`
	}
)

const localFile = "Local File"

// id is the path relative to dir, which the desktop tool resolves from its
// working directory and the website generates for local-folder images
func (e entry) id() string { return "./" + e.path }

func (l *layout) marshal() ([]byte, error) {
	o := xmlOrder{
		Details: xmlDetails{Quantity: l.quantity, Stock: string(l.stock), Foil: l.foil},
		Fronts:  xmlFace{xmlCards(l.fronts)},
	}
	// Omit <backs> when empty, as the website does
	if len(l.backs) > 0 {
		o.Backs = &xmlFace{xmlCards(l.backs)}
	}
	// An empty <cardback/> makes the website import a blank cardback, where a
	// missing one falls back to the project's own, so it is omitted when unset
	if l.cardback != nil {
		o.Cardback = l.cardback.id()
	}
	out, err := xml.MarshalIndent(o, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(append([]byte(xml.Header), out...), '\n'), nil
}

func xmlCards(es []entry) []xmlCard {
	cards := make([]xmlCard, len(es))
	for i, e := range es {
		slots := make([]string, len(e.slots))
		for j, s := range e.slots {
			slots[j] = strconv.Itoa(s)
		}
		cards[i] = xmlCard{
			ID:         e.id(),
			SourceType: localFile,
			// Bare comma-separated integers: the desktop tool also accepts
			// "[0, 1]", the website does not
			Slots: strings.Join(slots, ","),
			Name:  path.Base(e.path),
			Query: e.query,
		}
	}
	return cards
}
