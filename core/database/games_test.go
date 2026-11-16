package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/playnux/core/models"
)

func TestListGamesAndGetGame(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	recent := time.Now().Add(-time.Hour)
	games := []*models.Game{
		{
			GameID:       "1",
			PluginID:     "steam",
			Source:       "Steam",
			Name:         "Zeta",
			IsInstalled:  true,
			Favorite:     true,
			Genres:       []string{"RPG"},
			Platforms:    []string{"PC (Linux)"},
			LastActivity: &recent,
		},
		{
			GameID:   "2",
			PluginID: "gog",
			Source:   "GOG",
			Name:     "Alpha",
			Hidden:   true,
		},
		{
			GameID:    "3",
			PluginID:  "epic",
			Source:    "Epic",
			Name:      "Beta",
			Platforms: []string{"PC (Windows)"},
		},
	}
	if err := db.UpsertGames(ctx, games); err != nil {
		t.Fatal(err)
	}

	all, err := db.ListGames(ctx, "all", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "Beta" || all[1].Name != "Zeta" {
		t.Fatalf("lista = %+v", all)
	}

	installed, err := db.ListGames(ctx, "installed", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0].Name != "Zeta" {
		t.Fatalf("instalados = %+v", installed)
	}

	favorites, err := db.ListGames(ctx, "favorites", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != 1 || favorites[0].GameID != "1" {
		t.Fatalf("favoritos = %+v", favorites)
	}
	if got := favorites[0].Genres; len(got) != 1 || got[0] != "RPG" {
		t.Fatalf("gêneros não decodificados: %v", got)
	}

	recentGames, err := db.ListGames(ctx, "recent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentGames) != 1 || recentGames[0].GameID != "1" {
		t.Fatalf("recentes = %+v", recentGames)
	}

	found, err := db.SearchGames(ctx, "Zeta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "Zeta" || !found[0].IsInstalled {
		t.Fatalf("busca = %+v", found)
	}

	game, err := db.GetGame(ctx, games[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	if game.Name != "Beta" || game.PluginID != "epic" {
		t.Fatalf("detalhe = %+v", game)
	}
	if _, err := db.GetGame(ctx, "missing"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("esperava ErrGameNotFound, obtive %v", err)
	}
}
