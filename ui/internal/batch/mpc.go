package batch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/odevine/mimic/engine/mpcfill"
)

// Project is a run's MPC Autofill project: the layout from mpcfill, and the
// file each row of the run is written to
type Project struct {
	layout *mpcfill.Layout
	files  []string
	// rows maps each row of this run to its row in the first run, and done
	// records which of those rendered in this run or one it retried. A file on
	// disk is not enough, since it may be left from an earlier render
	rows []int
	mu   sync.Mutex
	done []bool
	// cardback is the stored image Prepare copies in. A retry leaves it empty
	// and keeps the cardback its project started with
	cardback string
}

// ProjectSpec is what a project takes beside its rows. CardbackFile is the
// stored cardback image, named CardbackName in the project
type ProjectSpec struct {
	Stock        mpcfill.Stock
	Foil         bool
	CardbackName string
	CardbackFile string
}

// PlanProject lays rows out as an MPC Autofill project. Rows come from
// ExpandFaces, so a row of Face 1 is the back of the card before it, and the
// rest share the cardback. Its errors are written for the person who pressed
// Render
func PlanProject(rows []Row, spec ProjectSpec) (*Project, error) {
	var cards []mpcfill.Card
	card := make([]int, len(rows))
	total := 0
	for i, row := range rows {
		switch row.Face {
		case 0:
			cards = append(cards, mpcfill.Card{Front: mpcfill.Face{Name: row.displayName()}, Quantity: row.Qty})
			total += max(row.Qty, 1)
		case 1:
			if len(cards) == 0 || cards[len(cards)-1].Back != nil {
				return nil, fmt.Errorf("%s: a back face needs its front before it", row.displayName())
			}
			cards[len(cards)-1].Back = &mpcfill.Face{Name: row.displayName()}
		default:
			return nil, fmt.Errorf("%s: MPC Autofill prints two faces at most", row.displayName())
		}
		card[i] = len(cards) - 1
	}
	if total > mpcfill.MaxProjectSize {
		return nil, fmt.Errorf("an MPC Autofill project holds at most %d cards and this one has %d, so split the list", mpcfill.MaxProjectSize, total)
	}

	p := mpcfill.Project{Stock: spec.Stock, Foil: spec.Foil, Cards: cards}
	if spec.CardbackName != "" {
		p.Cardback = &mpcfill.Face{Name: spec.CardbackName}
	}
	l, err := mpcfill.Plan(p)
	if errors.Is(err, mpcfill.ErrNoCardback) {
		return nil, errors.New("choose a cardback for the MPC Autofill project first")
	}
	if err != nil {
		return nil, err
	}

	files := make([]string, len(rows))
	for i, row := range rows {
		f := l.Cards[card[i]].Front
		if row.Face == 1 {
			f = l.Cards[card[i]].Back
		}
		files[i] = filepath.FromSlash(f)
	}
	all := make([]int, len(rows))
	for i := range all {
		all[i] = i
	}
	return &Project{layout: l, files: files, rows: all, done: make([]bool, len(rows)), cardback: spec.CardbackFile}, nil
}

// Prepare makes the project's folders under dir, removes any order file an
// earlier render left there, and copies the cardback in
func (p *Project) Prepare(dir string) error {
	if err := os.Remove(filepath.Join(dir, mpcfill.OrderFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing the old %s: %w", mpcfill.OrderFile, err)
	}
	for _, sub := range []string{mpcfill.FrontsDir, mpcfill.BacksDir, mpcfill.CardbackDir} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return fmt.Errorf("creating the project folders: %w", err)
		}
	}
	if p.cardback == "" || p.layout.Cardback == "" {
		return nil
	}
	raw, err := os.ReadFile(p.cardback)
	if err != nil {
		return fmt.Errorf("reading the cardback: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, filepath.FromSlash(p.layout.Cardback)), raw, 0o644)
}

// retry is the project for a run of just the given rows of this one, which
// carries over what is already done
func (p *Project) retry(rows []int) *Project {
	files := make([]string, len(rows))
	orig := make([]int, len(rows))
	for i, r := range rows {
		files[i] = p.files[r]
		orig[i] = p.rows[r]
	}
	p.mu.Lock()
	done := append([]bool(nil), p.done...)
	p.mu.Unlock()
	return &Project{layout: p.layout, files: files, rows: orig, done: done}
}

// finish records which cards this run rendered and writes cards.xml once
// every card of the project is done
func (p *Project) finish(r *Run) {
	v := r.View()
	failed, missing := 0, 0
	p.mu.Lock()
	for i, c := range v.Cards {
		if c.Status == StatusDone {
			p.done[p.rows[i]] = true
		} else if c.Status == StatusFailed {
			failed++
		}
	}
	for _, d := range p.done {
		if !d {
			missing++
		}
	}
	p.mu.Unlock()

	if missing > 0 {
		// Retry renders only this run's failures, so it finishes the project
		// only when they are all that is missing
		fix := "render the list again"
		if missing == failed {
			fix = "retry the failed cards"
		}
		left := fmt.Sprintf("%d cards are", missing)
		if missing == 1 {
			left = "1 card is"
		}
		r.log(fmt.Sprintf("%s is written once every card is done, and %s not, so %s to finish the project", mpcfill.OrderFile, left, fix))
		return
	}
	if err := mpcfill.WriteOrder(r.opts.OutDir, p.layout); err != nil {
		r.log(mpcfill.OrderFile + " not written: " + err.Error())
		return
	}
	r.mu.Lock()
	r.order = mpcfill.OrderFile
	r.mu.Unlock()
	r.log("wrote " + mpcfill.OrderFile + ", so the folder is ready for MPC Autofill")
}
