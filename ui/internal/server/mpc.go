package server

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/odevine/mimic/engine/mpcfill"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// maxCardbackUpload bounds a cardback upload. The stored PNG must still come in
// under mpcfill.MaxImageBytes, but a JPEG can grow a lot when re-encoded
const maxCardbackUpload = 64 << 20

// mpcOptions is the MPC Autofill part of a run request. Its presence makes the
// run write a project rather than loose PNGs
type mpcOptions struct {
	Stock string `json:"stock"`
	Foil  bool   `json:"foil"`
}

// planProject lays the expanded rows out as a project with the stored
// cardback. A face no template renders would leave the project without its
// image for good, so a card with one is refused rather than skipped
func (s *Server) planProject(rows []batch.Row, opts mpcOptions) (*batch.Project, error) {
	for _, row := range rows {
		if err := s.pipe.Unsupported(cardlist.Overlay(&row.Base, row.Fields), row.Face); err != nil {
			return nil, fmt.Errorf("%s cannot go in an MPC Autofill project, since %v. Untick it to export the rest", row.Base.Name, err)
		}
	}
	spec := batch.ProjectSpec{Stock: mpcfill.Stock(opts.Stock), Foil: opts.Foil}
	if name, ok := s.cardback(); ok {
		spec.CardbackName = name
		spec.CardbackFile, _ = cardbackPath()
	}
	return batch.PlanProject(rows, spec)
}

// cardbackPath is where the uploaded cardback is kept, beside prefs.json
func cardbackPath() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mimic", "cardback.png"), nil
}

// cardback returns the stored cardback's name, and whether there is one
func (s *Server) cardback() (string, bool) {
	name := s.prefs.CardbackName()
	path, err := cardbackPath()
	if name == "" || err != nil {
		return "", false
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		return "", false
	}
	return name, true
}

// mpcInfo is what GET /api/mpc returns: the choices a project offers and the
// stored cardback
type mpcInfo struct {
	Stocks   []string      `json:"stocks"`
	MaxCards int           `json:"maxCards"`
	Cardback *cardbackInfo `json:"cardback"`
}

type cardbackInfo struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	DPI    int    `json:"dpi"`
}

func (s *Server) handleMPC(w http.ResponseWriter, r *http.Request) {
	info := mpcInfo{
		Stocks:   []string{string(mpcfill.S30), string(mpcfill.S27), string(mpcfill.S33), string(mpcfill.M31), string(mpcfill.P10)},
		MaxCards: mpcfill.MaxProjectSize,
	}
	if name, ok := s.cardback(); ok {
		path, _ := cardbackPath()
		if f, err := os.Open(path); err == nil {
			cfg, _, err := image.DecodeConfig(f)
			f.Close()
			if err == nil {
				info.Cardback = &cardbackInfo{Name: name, Width: cfg.Width, Height: cfg.Height, DPI: mpcfill.DPI(cfg.Height)}
			}
		}
	}
	writeJSON(w, info)
}

// handleCardbackImage serves the stored cardback for the options thumbnail
func (s *Server) handleCardbackImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cardback(); !ok {
		http.NotFound(w, r)
		return
	}
	path, _ := cardbackPath()
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// handlePutCardback stores an uploaded PNG or JPEG as the cardback, named by the
// name query parameter. It is kept as a PNG, since the project names it .png,
// and refused when the website would hide it
func (s *Server) handlePutCardback(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Cardback"
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCardbackUpload))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, fmt.Sprintf("the cardback must be under %d MB", maxCardbackUpload>>20), http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "reading the cardback: "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	// The header is enough to refuse an oversized image before decoding it
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		http.Error(w, "the cardback must be a PNG or JPEG image", http.StatusBadRequest)
		return
	}
	if dpi := mpcfill.DPI(cfg.Height); dpi > mpcfill.MaxDPI {
		http.Error(w, fmt.Sprintf("that image is %d DPI by MPC Autofill's measure, and its website hides anything over %d", dpi, mpcfill.MaxDPI), http.StatusBadRequest)
		return
	}
	// A card is portrait, which also bounds the width the decode allocates
	if cfg.Width > cfg.Height {
		http.Error(w, fmt.Sprintf("the cardback is %d×%d, and a card is taller than it is wide", cfg.Width, cfg.Height), http.StatusBadRequest)
		return
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		http.Error(w, "the cardback could not be read: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.cardbackMu.Lock()
	defer s.cardbackMu.Unlock()
	path, err := cardbackPath()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err != nil {
		http.Error(w, "storing the cardback: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Written beside the stored one first, so a refused upload keeps the old
	tmp := path + ".new"
	defer os.Remove(tmp)
	if err := batch.WritePNG(tmp, img); err != nil {
		http.Error(w, "storing the cardback: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if fi, err := os.Stat(tmp); err != nil || fi.Size() > mpcfill.MaxImageBytes {
		http.Error(w, fmt.Sprintf("that image is over %d MB as a PNG, and MPC Autofill's website hides anything larger", mpcfill.MaxImageBytes/1_000_000), http.StatusBadRequest)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		http.Error(w, "storing the cardback: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.prefs.SetCardbackName(name)
	s.handleMPC(w, r)
}
