package batch

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PrepareOutDir resolves the output folder, creating it when it does not exist
// yet, and checks it can be written before any card renders
func PrepareOutDir(dir string) (string, error) {
	dir, err := ExpandPath(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating the output folder: %w", err)
	}
	probe, err := os.CreateTemp(dir, ".mimic-probe-*")
	if err != nil {
		return "", fmt.Errorf("the output folder is not writable: %w", err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return dir, nil
}

// ExpandPath turns a typed path into a clean absolute one, reading a leading ~
// as the home folder. A relative path is refused, since relative to the
// server's working directory is never what a user means
func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("choose an output folder")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%q is not a full path", p)
	}
	return filepath.Clean(p), nil
}

// outputNames picks every card's filename, ending in ext, up front, in list order, so two cards
// that would share a name get the same suffix however the workers interleave.
// A name carries the set code and collector number of the printing pulled from
// Scryfall, and a rerun into the same folder overwrites the files it wrote
func outputNames(rows []Row, ext string) []string {
	names := make([]string, len(rows))
	seen := make(map[string]int)
	for i, row := range rows {
		stem := fileStem(row.displayName(), row.Base.SetCode, row.Base.CollectorNumber)
		key := strings.ToLower(stem)
		seen[key]++
		if n := seen[key]; n > 1 {
			stem = fmt.Sprintf("%s (%d)", stem, n)
		}
		names[i] = stem + ext
	}
	return names
}

// fileStem builds a filename without its extension: the card name, then the
// printing in brackets when there is one, as in Sol Ring [C21-263]
func fileStem(name, set, number string) string {
	stem := sanitizeFilename(name)
	if stem == "" {
		stem = "Untitled"
	}
	set, number = sanitizeFilename(set), sanitizeFilename(number)
	switch {
	case set != "" && number != "":
		stem += fmt.Sprintf(" [%s-%s]", strings.ToUpper(set), number)
	case set != "":
		stem += fmt.Sprintf(" [%s]", strings.ToUpper(set))
	}
	return stem
}

// windowsReserved are names Windows refuses for a file whatever the extension
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeFilename makes a card name safe as a filename on every OS. A split
// card's // becomes a dash, characters no filesystem accepts are dropped, and
// the result is bounded in length
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, " // ", " - ")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(strings.TrimSpace(b.String()), ". ")
	if len(out) > 150 {
		out = strings.TrimSpace(out[:150])
	}
	if windowsReserved[strings.ToUpper(out)] {
		out = "_" + out
	}
	return out
}

// Compression reads the PNG compression setting, "fast" for the encoder's
// fastest level and anything else for its default
func Compression(setting string) png.CompressionLevel {
	if setting == "fast" {
		return png.BestSpeed
	}
	return png.DefaultCompression
}

// The image formats a run writes. A zero value reads as JPEG, the default
const (
	FormatJPEG = "jpeg"
	FormatPNG  = "png"
)

// DefaultJPEGQuality is the quality a card's JPEG is written at
const DefaultJPEGQuality = 95

// ParseFormat reads the image format setting, "png" for PNG and anything else
// for the default, JPEG
func ParseFormat(setting string) string {
	if setting == FormatPNG {
		return FormatPNG
	}
	return FormatJPEG
}

// Ext is the file extension, with its dot, a format's images carry
func Ext(format string) string {
	if format == FormatPNG {
		return ".png"
	}
	return ".jpg"
}

// WriteJPEG encodes img to path at quality through a temporary file in the same
// folder, so a crash or a Stop never leaves a half-written JPEG under the real name
func WriteJPEG(path string, img image.Image, quality int) error {
	return writeAtomic(path, ".mimic-*.jpg", func(w io.Writer) error {
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	})
}

// WritePNG encodes img to path through a temporary file in the same folder, so
// a crash or a Stop never leaves a half-written PNG under the real name
func WritePNG(path string, img image.Image, level png.CompressionLevel) error {
	return writeAtomic(path, ".mimic-*.png", func(w io.Writer) error {
		enc := png.Encoder{CompressionLevel: level}
		return enc.Encode(w, img)
	})
}

// writeAtomic runs encode into a temporary file beside path and renames it into
// place once it is complete
func writeAtomic(path, pattern string, encode func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	bw := bufio.NewWriterSize(tmp, 1<<20)
	if err := encode(bw); err != nil {
		tmp.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file private, and output is an ordinary user file
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Report is the JSON written beside the images, enough to see what a run did and
// to reproduce it
type Report struct {
	ID          string         `json:"id"`
	Label       string         `json:"label,omitempty"`
	Started     time.Time      `json:"started"`
	Finished    time.Time      `json:"finished"`
	Stopped     bool           `json:"stopped,omitempty"`
	Template    string         `json:"template"`
	DPI         int            `json:"dpi"`
	Format      string         `json:"format"`
	Concurrency int            `json:"concurrency"`
	Counts      map[string]int `json:"counts"`
	Order       string         `json:"order,omitempty"`
	// Warmup and WarmupCards are as in View. The rate of the rest of the run is
	// the cards after the first WarmupCards over the time after Warmup
	Warmup      int64        `json:"warmupMs,omitempty"`
	WarmupCards int          `json:"warmupCards,omitempty"`
	Cards       []ReportCard `json:"cards"`
}

// ReportCard is one card's line in a Report
type ReportCard struct {
	Name      string            `json:"name"`
	Qty       int               `json:"qty"`
	Group     string            `json:"group,omitempty"`
	Set       string            `json:"set,omitempty"`
	Collector string            `json:"collector,omitempty"`
	Face      int               `json:"face,omitempty"`
	Status    string            `json:"status"`
	Stage     string            `json:"stage,omitempty"`
	Error     string            `json:"error,omitempty"`
	File      string            `json:"file,omitempty"`
	Millis    int64             `json:"ms,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// writeReport writes mimic-run-<timestamp>.json into the run's folder
func writeReport(r *Run) (string, error) {
	v := r.View()
	rep := Report{
		ID:          v.ID,
		Label:       v.Label,
		Started:     v.Started,
		Finished:    v.Finished,
		Stopped:     v.Stopped,
		Template:    v.Template,
		DPI:         v.DPI,
		Format:      v.Format,
		Concurrency: v.Concurrency,
		Counts:      Counts(v.Cards),
		Order:       v.Order,
		Warmup:      v.Warmup,
		WarmupCards: v.WarmupCards,
	}
	for i, c := range v.Cards {
		row := r.rows[i]
		rep.Cards = append(rep.Cards, ReportCard{
			Name:      c.Name,
			Qty:       max(row.Qty, 1),
			Group:     row.Group,
			Set:       row.Base.SetCode,
			Collector: row.Base.CollectorNumber,
			Face:      row.Face,
			Status:    c.Status,
			Stage:     c.Stage,
			Error:     c.Err,
			File:      c.File,
			Millis:    c.Millis,
			Fields:    row.Fields,
		})
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.opts.OutDir, "mimic-run-"+v.Started.Format("20060102-150405")+".json")
	return path, os.WriteFile(path, raw, 0o644)
}
