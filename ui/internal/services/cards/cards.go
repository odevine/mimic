// Package cards is the service behind the search box and the printing picker
package cards

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Service searches Scryfall, or the local copy of its data, for cards
type Service struct{ ws *workspace.Workspace }

// New returns the service over a workspace
func New(ws *workspace.Workspace) *Service { return &Service{ws: ws} }

// SearchResult is one row the search returns: the display line the list shows
// and the full card the client fills its form from and sends back
type SearchResult struct {
	Text string               `json:"text"`
	Card *cardlist.ShapedCard `json:"card"`
}

// Search runs a Scryfall search and returns the matches. A successful search is
// remembered for the suggestion list. A cancelled search returns the context's
// error
func (s *Service) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchResult{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, workspace.NetTimeout)
	defer cancel()

	found, err := s.ws.Pipe.Client().Search(ctx, query)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, apierr.Wrap(apierr.BadGateway, "search failed", err)
	}
	s.ws.RememberQuery(query)

	out := make([]SearchResult, len(found))
	for i, c := range found {
		out[i] = SearchResult{Text: rowText(c), Card: cardlist.Shaped(c)}
	}
	return out, nil
}

// printingsQuery is the Scryfall search listing every printing of one card by
// exact name, newest first
func printingsQuery(name string) string {
	return fmt.Sprintf("!%q unique:prints order:released dir:desc", name)
}

// Printings lists every printing of a card, for the editor's printing picker. It
// reads the local copy when that is in use, and otherwise goes through Search
// rather than a set-and-number fetch, so it needs nothing the card client does
// not already do
func (s *Service) Printings(ctx context.Context, name string) ([]*cardlist.ShapedCard, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []*cardlist.ShapedCard{}, nil
	}
	// A card newer than the local copy is not in it, so an empty answer still
	// asks Scryfall
	if s.ws.UseLocalCards() {
		if found, err := s.ws.Cards.Printings(name); err == nil && len(found) > 0 {
			return cardlist.ShapedAll(found), nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, workspace.NetTimeout)
	defer cancel()

	found, err := s.ws.Pipe.Client().Search(ctx, printingsQuery(name))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, apierr.Wrap(apierr.BadGateway, "listing printings failed", err)
	}
	if found == nil {
		found = []*card.Data{}
	}
	return cardlist.ShapedAll(found), nil
}

// Recents returns the recent-search suggestions, newest first
func (s *Service) Recents() []string { return s.ws.RecentQueries() }

// rowText is one result-list line: the card name, its set, and its type
func rowText(d *card.Data) string {
	parts := []string{d.Name}
	if d.SetCode != "" {
		parts = append(parts, strings.ToUpper(d.SetCode))
	}
	if d.TypeLine != "" {
		parts = append(parts, d.TypeLine)
	}
	return strings.Join(parts, "  ·  ")
}
