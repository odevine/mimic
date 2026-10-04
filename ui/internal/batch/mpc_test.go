package batch

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/mpcfill"
)

func faceRow(name string, face, qty int) Row {
	return Row{Qty: qty, Face: face, Base: card.Data{Name: name, Faces: []card.Face{{Name: name}, {Name: name + " Back"}}}}
}

func TestPlanProjectPairsFaces(t *testing.T) {
	rows := []Row{faceRow("Delver", 0, 2), faceRow("Delver", 1, 2), faceRow("Island", 0, 4)}
	p, err := PlanProject(rows, ProjectSpec{CardbackName: "Back"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fronts/Delver.png", "backs/Delver Back.png", "fronts/Island.png"}
	got := make([]string, len(p.files))
	for i, f := range p.files {
		got[i] = filepath.ToSlash(f)
	}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v want %v", got, want)
	}
	if p.layout.Cardback != "cardback/Back.png" {
		t.Errorf("cardback = %q", p.layout.Cardback)
	}
}

func TestPlanProjectRefusals(t *testing.T) {
	cases := []struct {
		name string
		rows []Row
		spec ProjectSpec
		want string
	}{
		{"back first", []Row{faceRow("A", 1, 1)}, ProjectSpec{CardbackName: "B"}, "needs its front"},
		{"third face", []Row{faceRow("A", 0, 1), faceRow("A", 2, 1)}, ProjectSpec{CardbackName: "B"}, "two faces"},
		{"no cardback", []Row{faceRow("A", 0, 1)}, ProjectSpec{}, "choose a cardback"},
		// The back's copies are the front's, so only fronts count toward 612
		{"too many", []Row{faceRow("A", 0, 600), faceRow("A", 1, 600), faceRow("B", 0, 13)}, ProjectSpec{CardbackName: "B"}, "has 613"},
	}
	for _, c := range cases {
		if _, err := PlanProject(c.rows, c.spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v want %q", c.name, err, c.want)
		}
	}
	if _, err := PlanProject([]Row{faceRow("A", 0, 612)}, ProjectSpec{CardbackName: "B"}); err != nil {
		t.Errorf("612 cards: %v", err)
	}
}

// projectRun plans rows as a project in dir and returns a run of it with its
// log lines collected, plus a helper that writes a planned file
func projectRun(t *testing.T, dir string, rows []Row) (*Run, *[]string, func(string)) {
	t.Helper()
	p, err := PlanProject(rows, ProjectSpec{CardbackName: "Back"})
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	r := New(rows, Options{OutDir: dir, Project: p, Emit: func(e Event) {
		if e.Log != "" {
			logs = append(logs, e.Log)
		}
	}})
	write := func(rel string) {
		name := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(name), 0o755)
		os.WriteFile(name, []byte("png"), 0o644)
	}
	return r, &logs, write
}

func lastLog(logs *[]string) string {
	if len(*logs) == 0 {
		return ""
	}
	return (*logs)[len(*logs)-1]
}

// retryRun starts a run of r's failed cards the way the server does
func retryRun(r *Run) *Run {
	rows, p := r.Retry()
	return New(rows, Options{OutDir: r.opts.OutDir, Project: p, Emit: r.opts.Emit})
}

func TestRetryFinishesProject(t *testing.T) {
	dir := t.TempDir()
	r, logs, write := projectRun(t, dir, []Row{faceRow("Good", 0, 1), faceRow("Bad", 0, 1)})
	r.cards[0].Status, r.cards[1].Status = StatusDone, StatusFailed
	write("fronts/Good.png")
	write("cardback/Back.png")
	r.opts.Project.finish(r)
	if r.View().Order != "" || !strings.Contains(lastLog(logs), "retry the failed cards") {
		t.Fatalf("after a failure: order %q, log %q", r.View().Order, lastLog(logs))
	}

	rr := retryRun(r)
	if f := filepath.ToSlash(rr.files[0]); len(rr.files) != 1 || f != "fronts/Bad.png" || rr.opts.Project.cardback != "" {
		t.Fatalf("retry writes %v", rr.files)
	}
	write("fronts/Bad.png")
	rr.cards[0].Status = StatusDone
	rr.opts.Project.finish(rr)
	if rr.View().Order != mpcfill.OrderFile {
		t.Errorf("retry did not write the order: %q", lastLog(logs))
	}
}

func TestRetryOfARetry(t *testing.T) {
	dir := t.TempDir()
	r, _, write := projectRun(t, dir, []Row{faceRow("A", 0, 1), faceRow("B", 0, 1), faceRow("C", 0, 1)})
	for _, f := range []string{"fronts/A.png", "fronts/B.png", "fronts/C.png", "cardback/Back.png"} {
		write(f)
	}
	r.cards[0].Status, r.cards[1].Status, r.cards[2].Status = StatusDone, StatusFailed, StatusFailed
	r.opts.Project.finish(r)

	rr := retryRun(r)
	rr.cards[0].Status, rr.cards[1].Status = StatusFailed, StatusDone
	rr.opts.Project.finish(rr)
	if rr.View().Order != "" {
		t.Fatal("order written with B still failed")
	}
	rrr := retryRun(rr)
	if len(rrr.files) != 1 || filepath.ToSlash(rrr.files[0]) != "fronts/B.png" {
		t.Fatalf("second retry writes %v", rrr.files)
	}
	rrr.cards[0].Status = StatusDone
	rrr.opts.Project.finish(rrr)
	if rrr.View().Order != mpcfill.OrderFile {
		t.Error("second retry did not write the order")
	}
}

// A file left by an earlier render into the same folder must not stand in
// for a card that failed this time
func TestFinishIgnoresFilesFromEarlierRenders(t *testing.T) {
	dir := t.TempDir()
	r, logs, write := projectRun(t, dir, []Row{faceRow("A", 0, 1), faceRow("B", 0, 1)})
	for _, f := range []string{"fronts/A.png", "fronts/B.png", "cardback/Back.png", mpcfill.OrderFile} {
		write(f)
	}
	if err := r.opts.Project.Prepare(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, mpcfill.OrderFile)); !os.IsNotExist(err) {
		t.Error("Prepare kept the old order file")
	}
	r.cards[0].Status, r.cards[1].Status = StatusDone, StatusFailed
	r.opts.Project.finish(r)
	if r.View().Order != "" {
		t.Errorf("order written over a failed card: %q", lastLog(logs))
	}
}

// Retry renders only failures, so after a Stop the log must not promise it
// finishes the project
func TestFinishAfterAStopSaysRenderAgain(t *testing.T) {
	dir := t.TempDir()
	r, logs, _ := projectRun(t, dir, []Row{faceRow("A", 0, 1), faceRow("B", 0, 1), faceRow("C", 0, 1)})
	r.cards[0].Status, r.cards[1].Status, r.cards[2].Status = StatusDone, StatusFailed, StatusSkipped
	r.opts.Project.finish(r)
	if !strings.Contains(lastLog(logs), "2 cards are not, so render the list again") {
		t.Errorf("log = %q", lastLog(logs))
	}
	rr := retryRun(r)
	rr.cards[0].Status = StatusDone
	rr.opts.Project.finish(rr)
	if rr.View().Order != "" || !strings.Contains(lastLog(logs), "1 card is not, so render the list again") {
		t.Errorf("after retrying the failure: order %q, log %q", rr.View().Order, lastLog(logs))
	}
}

func TestExistingFiles(t *testing.T) {
	dir := t.TempDir()
	if n := ExistingFiles(filepath.Join(dir, "missing")); n != 0 {
		t.Errorf("missing folder: %d", n)
	}
	for _, f := range []string{"notes.txt", mpcfill.OrderFile, "fronts/A.png", "fronts/.DS_Store", "backs/B.png", "cardback/C.png", "other/D.png"} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755)
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
	// Only the order file and the images in the project's own subfolders count
	if n := ExistingFiles(dir); n != 4 {
		t.Errorf("ExistingFiles = %d, want 4", n)
	}
}
