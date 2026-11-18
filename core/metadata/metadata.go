// Package metadata enriquece os jogos com informações do IGDB e artes do
// SteamGridDB. Os provedores são opcionais: sem chaves configuradas a
// biblioteca continua funcionando e a UI informa o que falta.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/playnux/core/models"
)

// ErrNotFound indica que a busca não encontrou um jogo correspondente.
var ErrNotFound = errors.New("jogo não encontrado no provedor de metadados")

// Info representa os metadados ricos encontrados por nome.
type Info struct {
	Description    string
	ReleaseDate    *time.Time
	Genres         []string
	Tags           []string
	Developers     []string
	Publishers     []string
	CriticScore    *int
	CommunityScore *int
	Features       []string
	Platforms      []string
	TrailerURL     string
}

// Artwork contém as imagens verticais, horizontais e o logo transparente.
type Artwork struct {
	Cover      string
	Background string
	Logo       string
	Icon       string
}

type InfoProvider interface {
	Lookup(ctx context.Context, name string) (*Info, error)
}

type ArtworkProvider interface {
	Lookup(ctx context.Context, name string) (*Artwork, error)
}

// Service combina as duas fontes. Qualquer uma pode ser nil.
type Service struct {
	Info InfoProvider
	Art  ArtworkProvider
}

func (s *Service) Configured() bool {
	return s != nil && (s.Info != nil || s.Art != nil)
}

// Enrich aplica no jogo os dados encontrados. Se um provedor falhar mas o
// outro responder, o resultado parcial é mantido e o erro é ignorado; só há
// erro quando nenhuma fonte conseguiu contribuir.
func (s *Service) Enrich(ctx context.Context, game *models.Game) error {
	return s.EnrichFields(ctx, game, nil)
}

// EnrichFields applies only enabled fields. A nil policy preserves the legacy behavior.
func (s *Service) EnrichFields(ctx context.Context, game *models.Game, policy map[string]string) error {
	if s == nil || game == nil {
		return fmt.Errorf("metadados: jogo inválido")
	}
	infoNeeded, artNeeded := false, false
	for _, field := range Fields {
		if policy == nil || policy[field.ID] == "replace" || (policy[field.ID] == "missing" && !field.present(game)) {
			if field.Provider == "igdb" {
				infoNeeded = true
			} else {
				artNeeded = true
			}
		}
	}
	if !infoNeeded && !artNeeded {
		return nil
	}
	if (!infoNeeded || s.Info == nil) && (!artNeeded || s.Art == nil) {
		return fmt.Errorf("metadados: configure IGDB_CLIENT_ID/IGDB_CLIENT_SECRET e STEAMGRIDDB_API_KEY")
	}

	var failures []error
	success := false
	updated := *game

	if infoNeeded && s.Info != nil {
		info, err := s.Info.Lookup(ctx, game.Name)
		if err != nil {
			failures = append(failures, fmt.Errorf("IGDB: %w", err))
		} else if info != nil {
			applyInfo(&updated, info)
			success = true
		}
	}
	if artNeeded && s.Art != nil {
		art, err := s.Art.Lookup(ctx, game.Name)
		if err != nil {
			failures = append(failures, fmt.Errorf("SteamGridDB: %w", err))
		} else if art != nil {
			applyArtwork(&updated, art)
			success = true
		}
	}

	if !success {
		if len(failures) == 0 {
			return ErrNotFound
		}
		return errors.Join(failures...)
	}
	if policy != nil {
		for _, field := range Fields {
			if policy[field.ID] != "replace" && (policy[field.ID] != "missing" || field.present(game)) {
				field.restore(&updated, game)
			}
		}
	}
	*game = updated
	return nil
}

func applyInfo(game *models.Game, info *Info) {
	if strings.TrimSpace(info.Description) != "" {
		game.Description = info.Description
	}
	if info.ReleaseDate != nil {
		game.ReleaseDate = info.ReleaseDate
		game.ReleaseYear = info.ReleaseDate.Year()
	}
	if len(info.Genres) > 0 {
		game.Genres = info.Genres
	}
	if len(info.Tags) > 0 {
		game.Tags = info.Tags
	}
	if len(info.Developers) > 0 {
		game.Developers = info.Developers
	}
	if len(info.Publishers) > 0 {
		game.Publishers = info.Publishers
	}
	if info.CriticScore != nil {
		score := *info.CriticScore
		game.CriticScore = &score
	}
	if info.CommunityScore != nil {
		score := *info.CommunityScore
		game.CommunityScore = &score
	}
	if len(info.Features) > 0 {
		game.Features = info.Features
	}
	if len(info.Platforms) > 0 {
		game.Platforms = info.Platforms
	}
	if info.TrailerURL != "" {
		game.TrailerURL = info.TrailerURL
	}
}

func applyArtwork(game *models.Game, art *Artwork) {
	if art.Cover != "" {
		game.CoverImage = art.Cover
	}
	if art.Background != "" {
		game.BackgroundImage = art.Background
	}
	if art.Icon != "" {
		game.Icon = art.Icon
	}
	if art.Logo != "" {
		game.LogoImage = art.Logo
	}
}
