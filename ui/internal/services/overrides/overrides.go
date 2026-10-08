// Package overrides is the service behind the Overrides tab: the global rules
// that apply to every render, and the presets that save them with the render
// settings
package overrides

import (
	"reflect"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/rules"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// maxPresets and maxPresetName keep the saved presets few enough to list
const (
	maxPresets    = 50
	maxPresetName = 60
)

// Service reads and writes the global rules and the presets
type Service struct{ ws *workspace.Workspace }

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// Rules returns the global rules, in the order they apply
func (s *Service) Rules() []rules.Rule {
	if list := s.ws.Prefs.Rules(); list != nil {
		return list
	}
	return []rules.Rule{}
}

// SaveRules checks the rules and stores them, replacing the old list, and
// returns what was kept. A rule list that does not check is refused whole, with
// the first problem named
func (s *Service) SaveRules(list []rules.Rule) ([]rules.Rule, error) {
	if err := rules.Validate(list); err != nil {
		return nil, apierr.Wrap(apierr.BadRequest, "", err)
	}
	s.ws.Prefs.SetRules(list)
	return s.Rules(), nil
}

// RowRef is a list row as a rule sees it: the card it matched and the fields the
// row itself sets
type RowRef struct {
	Base   card.Data         `json:"base"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Matches counts, for each rule, how many of the rows its conditions hold for. A
// rule is tested on its own against the card as the row sets it, before any other
// rule, so the count says what the rule would catch and does not depend on the
// order. A disabled rule is counted as if it were on. The rules need not be
// saved, which is how the editor shows a count as a rule is typed
func (s *Service) Matches(list []rules.Rule, rows []RowRef) ([]int, error) {
	if err := rules.Validate(list); err != nil {
		return nil, apierr.Wrap(apierr.BadRequest, "", err)
	}
	counts := make([]int, len(list))
	values := make([]map[string]string, len(rows))
	for i := range rows {
		values[i] = fieldValues(&rows[i])
	}
	for i, r := range list {
		r.Disabled = false
		for _, v := range values {
			if r.Applies(v) {
				counts[i]++
			}
		}
	}
	return counts, nil
}

// Apply runs the saved rules over a card the way a render would and returns what
// they changed and which rules did it, for an editor to start from. fields are
// the ones the user has set on the card, which the rules leave alone and read
// when they test it. The card itself is not changed, so the editor still knows
// what was fetched
func (s *Service) Apply(base card.Data, fields map[string]string) rules.Result {
	return rules.ApplyToCard(s.ws.Prefs.Rules(), &base, fields)
}

// Preset is a saved preset as the menu lists it. Current is true when the rules
// and render settings now in force are exactly the ones it holds
type Preset struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Rules   int    `json:"rules"`
	Current bool   `json:"current"`
}

// Presets lists the saved presets
func (s *Service) Presets() []Preset {
	now := s.snapshot("")
	out := []Preset{}
	for _, p := range s.ws.Prefs.Presets() {
		out = append(out, info(p, now))
	}
	return out
}

func info(p prefs.Preset, now prefs.Preset) Preset {
	return Preset{Name: p.Name, Version: p.Version, Rules: len(p.Rules), Current: sameState(p, now)}
}

// snapshot is the rules and render settings now in force, as a preset
func (s *Service) snapshot(name string) prefs.Preset {
	settings := s.ws.Prefs.Settings()
	preview, output := s.ws.Prefs.Resolution()
	return prefs.Preset{
		Name:    name,
		Version: prefs.PresetVersion,
		Rules:   s.Rules(),
		Render: prefs.PresetRender{
			PreviewDPI:     workspace.PreviewOrDefault(preview),
			OutputDPI:      output,
			ImageFormat:    imageFormat(settings.ImageFormat),
			PNGCompression: compression(settings.PNGCompression),
			MPCStock:       settings.MPCStock,
			MPCFoil:        settings.MPCFoil,
		},
	}
}

// imageFormat and compression give the empty setting the value it reads as, so a
// preset made before the setting was touched equals one made after choosing the
// default
func imageFormat(f string) string {
	if f == "png" {
		return "png"
	}
	return "jpeg"
}

func compression(c string) string {
	if c == "fast" {
		return "fast"
	}
	return "balanced"
}

func sameState(a, b prefs.Preset) bool {
	return a.Render == b.Render && reflect.DeepEqual(normRules(a.Rules), normRules(b.Rules))
}

// normRules makes an empty list and a missing one equal
func normRules(list []rules.Rule) []rules.Rule {
	if len(list) == 0 {
		return nil
	}
	return list
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", apierr.New(apierr.BadRequest, "a preset needs a name")
	case len(name) > maxPresetName:
		return "", apierr.Newf(apierr.BadRequest, "a preset name is at most %d characters", maxPresetName)
	}
	return name, nil
}

// SavePreset saves the rules and render settings now in force under a name,
// replacing a preset of that name
func (s *Service) SavePreset(name string) (Preset, error) {
	name, err := cleanName(name)
	if err != nil {
		return Preset{}, err
	}
	have := s.ws.Prefs.Presets()
	exists := false
	for _, p := range have {
		exists = exists || strings.EqualFold(p.Name, name)
	}
	if !exists && len(have) >= maxPresets {
		return Preset{}, apierr.Newf(apierr.Conflict, "there are already %d presets, so delete one first", maxPresets)
	}
	p := s.snapshot(name)
	s.ws.Prefs.SetPreset(p)
	return info(p, p), nil
}

// Applied is what ApplyPreset changed, for the page to show: the rules now in
// force and the interface settings with the render settings in them
type Applied struct {
	Rules    []rules.Rule   `json:"rules"`
	Settings prefs.Settings `json:"settings"`
}

// ApplyPreset puts a preset's rules and render settings in force. Everything else
// in the settings, such as the theme and the output folder, is left as it is. A
// resolution the active template cannot reach is brought inside what it can
func (s *Service) ApplyPreset(name string) (Applied, error) {
	var p *prefs.Preset
	for _, have := range s.ws.Prefs.Presets() {
		if strings.EqualFold(have.Name, strings.TrimSpace(name)) {
			have := have
			p = &have
		}
	}
	if p == nil {
		return Applied{}, apierr.New(apierr.NotFound, "no such preset")
	}
	if p.Version > prefs.PresetVersion {
		return Applied{}, apierr.New(apierr.BadRequest, "this preset was saved by a newer version of Mimic")
	}
	if err := rules.Validate(p.Rules); err != nil {
		return Applied{}, apierr.Wrap(apierr.BadRequest, "this preset's rules do not check", err)
	}
	s.ws.Prefs.SetRules(p.Rules)
	settings := s.ws.Prefs.Settings()
	settings.ImageFormat, settings.PNGCompression = p.Render.ImageFormat, p.Render.PNGCompression
	settings.MPCStock, settings.MPCFoil = p.Render.MPCStock, p.Render.MPCFoil
	s.ws.Prefs.SetSettings(settings)
	preview, output := workspace.PreviewOrDefault(p.Render.PreviewDPI), p.Render.OutputDPI
	if m, err := s.ws.Pipe.Manifest(); err == nil {
		preview, output = m.ClampDPI(preview), workspace.ClampOutputDPI(m, output)
	}
	s.ws.Prefs.SetResolution(preview, output)
	return Applied{Rules: s.Rules(), Settings: s.ws.Prefs.Settings()}, nil
}

// DeletePreset removes a preset
func (s *Service) DeletePreset(name string) error {
	if !s.ws.Prefs.DeletePreset(strings.TrimSpace(name)) {
		return apierr.New(apierr.NotFound, "no such preset")
	}
	return nil
}

// fieldValues is a row's card with its own fields on it, as the strings a
// condition reads
func fieldValues(r *RowRef) map[string]string {
	return rules.FieldValuesOf(&r.Base, r.Fields)
}
