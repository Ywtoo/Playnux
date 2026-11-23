package epic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/playnux/core/models"
)

// Epic's ownership records may contain only catalog identifiers, even when
// metadata was requested. Resolve these in parallel with a bounded worker pool.
func (e *EpicProvider) resolveCatalog(ctx context.Context, records []epicRecord) ([]*models.Game, error) {
	if len(records) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pending := make(chan int, len(records))
	for i := range records {
		pending <- i
	}
	close(pending)

	resolved := make([]*models.Game, len(records))
	var wg sync.WaitGroup
	var once sync.Once
	var firstError error
	for workers := min(8, len(records)); workers > 0; workers-- {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range pending {
				if ctx.Err() != nil {
					return
				}
				record := records[i]
				item, err := e.fetchCatalogItem(ctx, record)
				if err != nil {
					once.Do(func() {
						firstError = err
						cancel()
					})
					return
				}
				if item != nil {
					record.CatalogItem = item
					resolved[i] = gameFromRecord(record)
				}
			}
		}()
	}
	wg.Wait()
	if firstError != nil {
		return nil, firstError
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var games []*models.Game
	for _, game := range resolved {
		if game != nil {
			games = append(games, game)
		}
	}
	return games, nil
}

func (e *EpicProvider) fetchCatalogItem(ctx context.Context, record epicRecord) (*epicCatalog, error) {
	id := strings.TrimSpace(record.CatalogItemID)
	query := url.Values{
		"id":                     {id},
		"includeDLCDetails":      {"true"},
		"includeMainGameDetails": {"true"},
		"country":                {strings.TrimSpace(record.Country)},
		"locale":                 {"en"},
	}
	endpoint := e.catalogBase + "/catalog/api/shared/namespace/" +
		url.PathEscape(strings.TrimSpace(record.Namespace)) + "/bulk/items?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+e.auth.AccessToken)
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Epic catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Epic catalog returned HTTP %d", resp.StatusCode)
	}
	var items map[string]*epicCatalog
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("Epic catalog: %w", err)
	}
	return items[id], nil
}
