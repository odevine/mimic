package run

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/odevine/mimic/engine/mpcfill"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// MaxCardbackUpload bounds a cardback upload. The stored PNG must still come in
// under mpcfill.MaxImageBytes, but a JPEG can grow a lot when re-encoded
const MaxCardbackUpload = 64 << 20

// maxCardbackHeight is the tallest cardback at mpcfill.MaxDPI, where DPI measures
// a card 1110 pixels tall at 300. A taller upload is scaled down to it
const maxCardbackHeight = mpcfill.MaxDPI * 1110 / 300

// maxCardbackPixels bounds what an upload decodes before it is scaled down, which
// is about 2500 DPI on a card
const maxCardbackPixels = 64_000_000

// MPCOptions is the MPC Autofill part of a run request. Its presence makes the
// run write a project rather than loose images
type MPCOptions struct {
	Stock string `json:"stock"`
	Foil  bool   `json:"foil"`
}

// planProject lays the expanded rows out as a project with the stored
// cardback. A face no template renders would leave the project without its
// image for good, so a card with one is refused rather than skipped
func (s *Service) planProject(rows []batch.Row, opts MPCOptions) (*batch.Project, error) {
	for _, row := range rows {
		if err := s.ws.Pipe.Unsupported(cardlist.Overlay(&row.Base, row.Fields), row.Face); err != nil {
			return nil, fmt.Errorf("%s cannot go in an MPC Autofill project, since %v. Untick it to export the rest", row.Base.Name, err)
		}
	}
	spec := batch.ProjectSpec{Stock: mpcfill.Stock(opts.Stock), Foil: opts.Foil}
	if name, ok := s.cardback(); ok {
		spec.CardbackName = name
		spec.CardbackFile, _ = workspace.CardbackPath()
	}
	return batch.PlanProject(rows, spec)
}

// cardback returns the stored cardback's name, and whether there is one
func (s *Service) cardback() (string, bool) {
	name := s.ws.Prefs.CardbackName()
	path, err := workspace.CardbackPath()
	if name == "" || err != nil {
		return "", false
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		return "", false
	}
	return name, true
}

// MPCInfo is what MPCInfo returns: the choices a project offers and the stored
// cardback
type MPCInfo struct {
	Stocks   []string      `json:"stocks"`
	MaxCards int           `json:"maxCards"`
	Cardback *CardbackInfo `json:"cardback"`
}

// CardbackInfo describes the stored cardback
type CardbackInfo struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	DPI    int    `json:"dpi"`
}

// MPCInfo returns the stocks a project offers and the stored cardback
func (s *Service) MPCInfo() MPCInfo {
	info := MPCInfo{
		Stocks:   []string{string(mpcfill.S30), string(mpcfill.S27), string(mpcfill.S33), string(mpcfill.M31), string(mpcfill.P10)},
		MaxCards: mpcfill.MaxProjectSize,
	}
	if name, ok := s.cardback(); ok {
		path, _ := workspace.CardbackPath()
		if f, err := os.Open(path); err == nil {
			cfg, _, err := image.DecodeConfig(f)
			f.Close()
			if err == nil {
				info.Cardback = &CardbackInfo{Name: name, Width: cfg.Width, Height: cfg.Height, DPI: mpcfill.DPI(cfg.Height)}
			}
		}
	}
	return info
}

// CardbackFile returns the path of the stored cardback for the options
// thumbnail, or a not found error when there is none
func (s *Service) CardbackFile() (string, error) {
	if _, ok := s.cardback(); !ok {
		return "", apierr.New(apierr.NotFound, "no cardback")
	}
	path, _ := workspace.CardbackPath()
	return path, nil
}

// SetCardback stores the PNG or JPEG at path as the cardback, named for its file
func (s *Service) SetCardback(path string) (MPCInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.BadRequest, "reading the cardback", err)
	}
	defer f.Close()
	return s.SetCardbackImage(filepath.Base(path), f)
}

// SetCardbackImage stores an image read from r as the cardback, under a name
// with any image extension removed. It is kept as a PNG, since the project names
// it .png, and scaled down or refused where the website would hide it
func (s *Service) SetCardbackImage(name string, r io.Reader) (MPCInfo, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg":
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Cardback"
	}
	raw, err := io.ReadAll(io.LimitReader(r, MaxCardbackUpload+1))
	if err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.BadRequest, "reading the cardback", err)
	}
	if len(raw) > MaxCardbackUpload {
		return MPCInfo{}, apierr.Newf(apierr.TooLarge, "the cardback must be under %d MB", MaxCardbackUpload>>20)
	}
	// The header is enough to refuse an oversized image before decoding it
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return MPCInfo{}, apierr.New(apierr.BadRequest, "the cardback must be a PNG or JPEG image")
	}
	if cfg.Width > cfg.Height {
		return MPCInfo{}, apierr.Newf(apierr.BadRequest, "the cardback is %d×%d, and a card is taller than it is wide", cfg.Width, cfg.Height)
	}
	if cfg.Width*cfg.Height > maxCardbackPixels {
		return MPCInfo{}, apierr.Newf(apierr.BadRequest, "the cardback is %d×%d, which is too large to scale down to MPC Autofill's %d DPI", cfg.Width, cfg.Height, mpcfill.MaxDPI)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.BadRequest, "the cardback could not be read", err)
	}
	if mpcfill.DPI(cfg.Height) > mpcfill.MaxDPI {
		img = template.Scale(float64(maxCardbackHeight) / float64(cfg.Height)).Image(img)
	}
	s.cardbackMu.Lock()
	defer s.cardbackMu.Unlock()
	path, err := workspace.CardbackPath()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.Internal, "storing the cardback", err)
	}
	// Written beside the stored one first, so a refused upload keeps the old
	tmp := path + ".new"
	defer os.Remove(tmp)
	if err := batch.WritePNG(tmp, img, png.DefaultCompression); err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.Internal, "storing the cardback", err)
	}
	if fi, err := os.Stat(tmp); err != nil || fi.Size() > mpcfill.MaxImageBytes {
		return MPCInfo{}, apierr.Newf(apierr.BadRequest, "that image is over %d MB as a PNG, and MPC Autofill's website hides anything larger", mpcfill.MaxImageBytes/1_000_000)
	}
	if err := os.Rename(tmp, path); err != nil {
		return MPCInfo{}, apierr.Wrap(apierr.Internal, "storing the cardback", err)
	}
	s.ws.Prefs.SetCardbackName(name)
	return s.MPCInfo(), nil
}

// MPCFolder reports how many files an earlier project left in the path folder,
// so the list can warn before a render overwrites them
func (s *Service) MPCFolder(path string) (map[string]int, error) {
	dir, err := batch.ExpandPath(path)
	if err != nil {
		return nil, apierr.Wrap(apierr.BadRequest, "", err)
	}
	return map[string]int{"existing": batch.ExistingFiles(dir)}, nil
}
