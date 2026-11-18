package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/playnux/core/models"
)

type fakeInfo struct {
	info *Info
	err  error
}

func (f fakeInfo) Lookup(context.Context, string) (*Info, error) { return f.info, f.err }

type fakeArt struct {
	art *Artwork
	err error
}

func (f fakeArt) Lookup(context.Context, string) (*Artwork, error) { return f.art, f.err }

func TestServiceAppliesPartialMetadata(t *testing.T) {
	release := time.Date(2020, 9, 17, 0, 0, 0, 0, time.UTC)
	score := 92
	service := &Service{
		Info: fakeInfo{info: &Info{
			Description: "Sinopse",
			ReleaseDate: &release,
			Genres:      []string{"RPG"},
			CriticScore: &score,
			TrailerURL:  "https://youtube/watch",
		}},
		Art: fakeArt{err: errors.New("sem arte")},
	}

	game := &models.Game{Name: "Hades"}
	if err := service.Enrich(context.Background(), game); err != nil {
		t.Fatalf("erro parcial não deveria falhar: %v", err)
	}
	if game.Description != "Sinopse" || game.ReleaseYear != 2020 || game.TrailerURL == "" {
		t.Fatalf("jogo = %+v", game)
	}
	if game.CoverImage != "" {
		t.Fatalf("capa não deveria ser alterada: %q", game.CoverImage)
	}
}

func TestServiceRequiresProvider(t *testing.T) {
	service := &Service{}
	if service.Configured() {
		t.Fatal("serviço vazio não deveria estar configurado")
	}
	if err := service.Enrich(context.Background(), &models.Game{Name: "X"}); err == nil {
		t.Fatal("esperava erro sem provedores")
	}
}

func TestEnrichFieldsFollowsPolicy(t *testing.T) {
	service := &Service{
		Info: fakeInfo{info: &Info{
			Description: "from provider",
			Genres:      []string{"Action"},
			Publishers:  []string{"Publisher"},
		}},
		Art: fakeArt{art: &Artwork{Cover: "new cover", Background: "new background"}},
	}
	game := &models.Game{Name: "Example", Description: "existing description", CoverImage: "old cover"}
	err := service.EnrichFields(context.Background(), game, map[string]string{
		"description": "missing", "genres": "missing", "publishers": "off",
		"cover": "replace", "background": "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if game.Description != "existing description" || len(game.Genres) != 1 ||
		len(game.Publishers) != 0 || game.CoverImage != "new cover" || game.BackgroundImage != "" {
		t.Fatalf("unexpected metadata: %+v", game)
	}
}

func TestPrioritizedSourcesUseFirstAvailableValue(t *testing.T) {
	service := &Service{
		Info: fakeInfo{info: &Info{Description: "IGDB summary", Tags: []string{"Fantasy"}}},
		Art:  fakeArt{art: &Artwork{Cover: "community cover", Icon: "community icon"}},
	}
	official := &models.Game{Description: "Official description", CoverImage: "official cover"}

	game := &models.Game{Name: "Game", Description: "Existing description"}
	if err := service.EnrichPrioritized(context.Background(), game, official, DefaultSources(), ModeMissing); err != nil {
		t.Fatal(err)
	}
	if game.Description != "Existing description" || game.CoverImage != "official cover" ||
		game.Icon != "community icon" || len(game.Tags) != 1 {
		t.Fatalf("modo preencher: %+v", game)
	}

	order := []string{SourceSteamGridDB, SourceStore, SourceIGDB}
	if err := service.EnrichPrioritized(context.Background(), game, official, order, ModeReplace); err != nil {
		t.Fatal(err)
	}
	if game.CoverImage != "community cover" || game.Description != "Official description" || game.Icon != "community icon" {
		t.Fatalf("modo substituir: %+v", game)
	}
}
