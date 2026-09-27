package cardlist

import (
	"context"
	"fmt"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/carddata"
)

// lookupBackend answers the questions resolving a list asks, from Scryfall's
// API or from the local copy of its bulk data. Each returns nil with no error
// when there is simply no such card
type lookupBackend interface {
	// query runs a Scryfall search, which only the API can answer
	query(ctx context.Context, q string) ([]*card.Data, error)
	// printing is the named card in a set, at a collector number when given
	printing(ctx context.Context, name, set, number string) (*card.Data, error)
	// exact is the card with exactly this name at its default printing
	exact(ctx context.Context, name string) (*card.Data, error)
	// fuzzy is the card a mistyped name most likely means
	fuzzy(ctx context.Context, name string) (*card.Data, error)
	// similar lists cards whose names contain every word, most played first
	similar(ctx context.Context, name string) ([]*card.Data, error)
}

// apiBackend answers from Scryfall's API through the card client
type apiBackend struct {
	src CardSource
}

func (b apiBackend) query(ctx context.Context, q string) ([]*card.Data, error) {
	return b.src.Search(ctx, q)
}

func (b apiBackend) printing(ctx context.Context, name, set, number string) (*card.Data, error) {
	q := fmt.Sprintf("!%q set:%s", name, set)
	if number != "" {
		q += " cn:" + number
	}
	cards, err := b.src.Search(ctx, q+" unique:prints")
	if err != nil || len(cards) == 0 {
		return nil, err
	}
	return exactName(cards, name), nil
}

func (b apiBackend) exact(ctx context.Context, name string) (*card.Data, error) {
	cards, err := b.src.Search(ctx, fmt.Sprintf("!%q", name))
	if err != nil || len(cards) == 0 {
		return nil, err
	}
	return exactName(cards, name), nil
}

func (b apiBackend) fuzzy(ctx context.Context, name string) (*card.Data, error) {
	d, err := b.src.FetchByName(ctx, name)
	if isNotFound(err) {
		return nil, nil
	}
	return d, err
}

func (b apiBackend) similar(ctx context.Context, name string) ([]*card.Data, error) {
	return b.src.Search(ctx, name+" order:edhrec")
}

// localBackend answers from the local copy. A Scryfall query still goes to the
// API, since the copy cannot run Scryfall's search syntax, and so does a
// printing the copy lacks, which is usually one newer than the download
type localBackend struct {
	store *carddata.Store
	api   apiBackend
}

func (b localBackend) query(ctx context.Context, q string) ([]*card.Data, error) {
	return b.api.query(ctx, q)
}

func (b localBackend) printing(ctx context.Context, name, set, number string) (*card.Data, error) {
	d, err := b.store.Printing(name, set, number)
	if d != nil || err != nil {
		return d, err
	}
	return b.api.printing(ctx, name, set, number)
}

func (b localBackend) exact(ctx context.Context, name string) (*card.Data, error) {
	return b.store.Exact(name)
}

func (b localBackend) fuzzy(ctx context.Context, name string) (*card.Data, error) {
	return b.store.Fuzzy(name)
}

func (b localBackend) similar(ctx context.Context, name string) ([]*card.Data, error) {
	return b.store.Similar(name, maxCandidates)
}

// Resolver is how a resolve looks rows up: a primary backend, and a fallback
// for a line the primary knows nothing about. Mode keys the session cache, so a
// result from one source is not served as the other's
type Resolver struct {
	mode     string
	primary  lookupBackend
	fallback lookupBackend
}

// APIResolver looks every row up through Scryfall's API
func APIResolver(src CardSource) Resolver {
	return Resolver{mode: "api", primary: apiBackend{src: src}}
}

// LocalResolver looks rows up in the local copy first, and asks the API about a
// line the copy knows nothing about, such as a card newer than the download
func LocalResolver(store *carddata.Store, src CardSource) Resolver {
	api := apiBackend{src: src}
	return Resolver{mode: "local", primary: localBackend{store: store, api: api}, fallback: api}
}
