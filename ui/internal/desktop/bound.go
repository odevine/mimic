package desktop

import (
	"context"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/carddata"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/rules"
	"github.com/odevine/mimic/ui/internal/services/cards"
	"github.com/odevine/mimic/ui/internal/services/data"
	"github.com/odevine/mimic/ui/internal/services/list"
	"github.com/odevine/mimic/ui/internal/services/overrides"
	"github.com/odevine/mimic/ui/internal/services/render"
	"github.com/odevine/mimic/ui/internal/services/run"
	"github.com/odevine/mimic/ui/internal/services/settings"
	"github.com/odevine/mimic/ui/internal/services/templates"
)

// Wails binds every exported method of a registered type, and the services have
// methods only the shell uses, such as the image lookups behind the asset
// handler. Each bound type below embeds an interface naming just the methods the
// page may call, so the page's reach is this file and nothing else. The names
// are what api.js calls, and a test keeps the two in step.

type cardsAPI interface {
	Search(ctx context.Context, query string) ([]cards.SearchResult, error)
	Printings(ctx context.Context, name string) ([]*cardlist.ShapedCard, error)
	Recents() []string
}

// Cards is the bound search service
type Cards struct{ cardsAPI }

type renderAPI interface {
	Start(req render.Request) (render.Started, error)
	Cancel(jobID string) error
	Save(jobID, path string) error
	SuggestedFilename(jobID string) (string, error)
}

// Render is the bound single card render service
type Render struct{ renderAPI }

type listAPI interface {
	Resolve(req list.Request) (list.Started, error)
	ReadFile(path string) (string, error)
}

// List is the bound list service
type List struct{ listAPI }

type runAPI interface {
	Start(req run.Request) (batch.View, error)
	Latest() *batch.View
	Stop(id string) error
	Retry(id string) (batch.View, error)
	OpenFolder(id string) error
	MPCInfo() run.MPCInfo
	MPCFolder(path string) (map[string]int, error)
	SetCardback(path string) (run.MPCInfo, error)
}

// Run is the bound batch run service
type Run struct {
	runAPI
	lifecycle
}

type templatesAPI interface {
	List(ctx context.Context) []templates.TemplateView
	Active() templates.ActiveView
	Select(req templates.SelectRequest) (string, error)
	Faces() []pipeline.FaceRow
	SetFace(c templates.FaceChoice) ([]pipeline.FaceRow, error)
}

// Templates is the bound template service
type Templates struct {
	templatesAPI
	lifecycle
}

type settingsAPI interface {
	Get() prefs.Settings
	Put(in prefs.Settings) prefs.Settings
	Resolution() (settings.Resolutions, error)
	SetResolution(req settings.ResolutionRequest) (settings.Resolutions, error)
	Resources(cache string) settings.ResourcesView
	Capabilities() map[string]settings.Gate
}

// Settings is the bound settings service
type Settings struct{ settingsAPI }

type overridesAPI interface {
	Rules() []rules.Rule
	SaveRules(list []rules.Rule) ([]rules.Rule, error)
	Matches(list []rules.Rule, rows []overrides.RowRef) ([]int, error)
	Apply(base card.Data, fields map[string]string) rules.Result
	Presets() []overrides.Preset
	SavePreset(name string) (overrides.Preset, error)
	ApplyPreset(name string) (overrides.Applied, error)
	DeletePreset(name string) error
}

// Overrides is the bound global rules and presets service
type Overrides struct{ overridesAPI }

type dataAPI interface {
	CardData(ctx context.Context) data.CardDataView
	DownloadCardData() (string, error)
	RemoveCardData() (carddata.Status, error)
	Fonts() data.FontsStatus
	AddFont(folder, path string) (data.FontsStatus, error)
	RemoveFont(folder string) (data.FontsStatus, error)
	OpenFonts() error
}

// Data is the bound card data and fonts service
type Data struct {
	dataAPI
	lifecycle
}
