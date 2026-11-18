package metadata

import (
	"context"
	"errors"
	"fmt"

	"github.com/playnux/core/models"
)

const (
	SourceStore       = "store"
	SourceIGDB        = "igdb"
	SourceSteamGridDB = "steamgriddb"
	ModeMissing       = "missing"
	ModeReplace       = "replace"
)

func DefaultSources() []string {
	return []string{SourceStore, SourceIGDB, SourceSteamGridDB}
}

// EnrichPrioritized takes each value from the first selected source that has it.
func (s *Service) EnrichPrioritized(ctx context.Context, game, official *models.Game, sources []string, mode string) error {
	pending := make(map[string]bool)
	for _, field := range Fields {
		if mode == ModeReplace || !field.present(game) {
			pending[field.ID] = true
		}
	}
	if len(pending) == 0 {
		return nil
	}
	var failures []error
	applied := false
	for _, source := range sources {
		if len(pending) == 0 {
			break
		}
		var result models.Game
		switch source {
		case SourceStore:
			if official == nil {
				continue
			}
			result = *official
		case SourceIGDB:
			if s.Info == nil || !needsSource(pending, source) {
				continue
			}
			info, err := s.Info.Lookup(ctx, game.Name)
			if err != nil {
				failures = append(failures, fmt.Errorf("IGDB: %w", err))
				continue
			}
			if info != nil {
				applyInfo(&result, info)
			}
		case SourceSteamGridDB:
			if s.Art == nil || !needsSource(pending, source) {
				continue
			}
			art, err := s.Art.Lookup(ctx, game.Name)
			if err != nil {
				failures = append(failures, fmt.Errorf("SteamGridDB: %w", err))
				continue
			}
			if art != nil {
				applyArtwork(&result, art)
			}
		default:
			continue
		}
		for _, field := range Fields {
			if pending[field.ID] && field.present(&result) {
				field.restore(game, &result)
				delete(pending, field.ID)
				applied = true
			}
		}
	}
	if !applied && len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}

func needsSource(pending map[string]bool, source string) bool {
	for _, field := range Fields {
		if field.Provider == source && pending[field.ID] {
			return true
		}
	}
	return false
}
