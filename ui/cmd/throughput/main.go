// Command throughput renders a list of cards through the app's own run service
// and reports cards per second, so a change to the run loop or the engine can be
// compared against an earlier build on the same machine. It uses the real config
// folder's templates and card data, writes only to -out, and needs no window
//
//	throughput -rows rows.json [-out dir] [-runs 2] [-format jpeg]
//
// The rows file is a JSON array of run rows, as the page sends them. A first run
// fetches art, so the later runs are the ones that measure rendering
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/workspace"
)

func main() {
	if err := measure(); err != nil {
		fmt.Fprintln(os.Stderr, "throughput:", err)
		os.Exit(1)
	}
}

func measure() error {
	rowsFile := flag.String("rows", "", "JSON file of run rows")
	out := flag.String("out", "", "folder for the rendered cards, a temporary one when empty")
	runs := flag.Int("runs", 2, "how many times to render the list")
	flag.Parse()
	if *rowsFile == "" {
		return fmt.Errorf("-rows is required")
	}
	buf, err := os.ReadFile(*rowsFile)
	if err != nil {
		return err
	}
	var rows []batch.Row
	if err := json.Unmarshal(buf, &rows); err != nil {
		return fmt.Errorf("reading %s: %w", *rowsFile, err)
	}
	dir := *out
	if dir == "" {
		if dir, err = os.MkdirTemp("", "mimic-throughput-"); err != nil {
			return err
		}
		defer os.RemoveAll(dir)
	}

	ws := workspace.New(workspace.Options{})
	defer ws.Close()
	svc := run.New(ws, func(string) error { return nil })
	defer svc.Shutdown()

	for n := 1; n <= *runs; n++ {
		started := time.Now()
		view, err := svc.Start(run.Request{Rows: rows, OutDir: dir, Label: fmt.Sprintf("throughput %d", n)})
		if err != nil {
			return err
		}
		for ws.Run.Active() {
			time.Sleep(20 * time.Millisecond)
		}
		wall := time.Since(started)
		cur, _ := ws.Run.Current()
		final := cur.View()
		status := map[string]int{}
		for _, c := range final.Cards {
			status[c.Status]++
		}
		fmt.Printf("run %d: %d cards in %.2fs, %.1f cards/s (%s, %d dpi, %d workers) %v\n",
			n, len(final.Cards), wall.Seconds(), float64(len(final.Cards))/wall.Seconds(), view.Format, view.DPI, view.Concurrency, status)
	}
	return nil
}
