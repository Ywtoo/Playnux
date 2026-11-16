package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/playnux/core/models"
)

func TestLocalLaunchFieldsAndTrailerRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	game := &models.Game{
		GameID:      "rom:/games/ps2/game.iso",
		PluginID:    "emulation",
		Source:      "Emulação",
		Name:        "Game",
		Executable:  "/usr/bin/pcsx2",
		Arguments:   []string{"/games/ps2/game.iso"},
		TrailerURL:  "https://youtube/watch",
		LogoImage:   "https://cdn/logo.png",
		Platforms:   []string{"PlayStation 2"},
		IsInstalled: true,
	}
	if err := db.UpsertGames(ctx, []*models.Game{game}); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Executable != game.Executable || len(loaded.Arguments) != 1 {
		t.Fatalf("comando = %q %v", loaded.Executable, loaded.Arguments)
	}
	if loaded.TrailerURL != game.TrailerURL || loaded.LogoImage != game.LogoImage {
		t.Fatalf("mídia = %+v", loaded)
	}
}

func TestOpenMigratesLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
CREATE TABLE games (
	id TEXT PRIMARY KEY, game_id TEXT NOT NULL, plugin_id TEXT NOT NULL,
	source TEXT NOT NULL, name TEXT NOT NULL, sorting_name TEXT NOT NULL,
	icon TEXT, cover_image TEXT, background_image TEXT,
	is_installed INTEGER NOT NULL DEFAULT 0, install_directory TEXT,
	install_size INTEGER DEFAULT 0, playtime INTEGER DEFAULT 0,
	play_count INTEGER DEFAULT 0, last_activity DATETIME,
	description TEXT, release_date DATETIME, release_year INTEGER, version TEXT,
	user_score INTEGER, critic_score INTEGER, community_score INTEGER,
	hidden INTEGER NOT NULL DEFAULT 0, favorite INTEGER NOT NULL DEFAULT 0,
	is_custom_game INTEGER NOT NULL DEFAULT 0,
	genres_json TEXT DEFAULT '[]', developers_json TEXT DEFAULT '[]',
	publishers_json TEXT DEFAULT '[]', tags_json TEXT DEFAULT '[]',
	categories_json TEXT DEFAULT '[]', features_json TEXT DEFAULT '[]',
	platforms_json TEXT DEFAULT '[]',
	added DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	modified DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(source, game_id)
);
INSERT INTO games (id, game_id, plugin_id, source, name, sorting_name)
VALUES ('legacy-id', 'legacy-1', 'steam', 'Steam', 'Legacy Game', 'Legacy Game');
`)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	game, err := db.GetGame(ctx, "legacy-id")
	if err != nil || game.Name != "Legacy Game" {
		t.Fatalf("GetGame = %+v, %v", game, err)
	}
	found, err := db.SearchGames(ctx, "Legacy", 10)
	if err != nil || len(found) != 1 || found[0].ID != "legacy-id" {
		t.Fatalf("SearchGames = %+v, %v", found, err)
	}
}

func TestGetGameAcceptsNullMigratedColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	game := &models.Game{GameID: "old", PluginID: "steam", Source: "Steam", Name: "Old"}
	if err := db.UpsertGames(ctx, []*models.Game{game}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.conn.ExecContext(ctx, `
UPDATE games SET logo_image = NULL, trailer_url = NULL, executable = NULL,
	arguments_json = NULL, description = NULL, install_directory = NULL
WHERE id = ?;`, game.ID); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Old" || loaded.TrailerURL != "" || loaded.Executable != "" {
		t.Fatalf("jogo migrado = %+v", loaded)
	}
}

func TestUpdateGameMetadataPreservesLibraryState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	game := &models.Game{
		GameID:      "42",
		PluginID:    "steam",
		Source:      "Steam",
		Name:        "Hades",
		IsInstalled: true,
		Favorite:    true,
		Playtime:    120,
	}
	if err := db.UpsertGames(ctx, []*models.Game{game}); err != nil {
		t.Fatal(err)
	}

	release := time.Date(2020, 9, 17, 0, 0, 0, 0, time.UTC)
	score := 92
	game.Description = "Sinopse rica"
	game.ReleaseDate = &release
	game.ReleaseYear = 2020
	game.CriticScore = &score
	game.Genres = []string{"RPG"}
	game.Features = []string{"Single player"}
	game.Platforms = []string{"PC"}
	game.CommunityScore = &score
	game.Icon = "https://cdn/icon.png"
	game.TrailerURL = "https://youtube/watch"
	game.CoverImage = "https://cdn/cover.jpg"
	if err := db.UpdateGameMetadata(ctx, game); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Description != "Sinopse rica" || loaded.ReleaseYear != 2020 ||
		loaded.CriticScore == nil || *loaded.CriticScore != 92 {
		t.Fatalf("metadados = %+v", loaded)
	}
	if loaded.TrailerURL != game.TrailerURL || loaded.CoverImage != game.CoverImage || loaded.Icon != game.Icon {
		t.Fatalf("mídia = %+v", loaded)
	}
	if len(loaded.Features) != 1 || len(loaded.Platforms) != 1 || loaded.CommunityScore == nil || *loaded.CommunityScore != score {
		t.Fatalf("ficha técnica = %+v", loaded)
	}
	if !loaded.IsInstalled || !loaded.Favorite || loaded.Playtime != 120 {
		t.Fatalf("estado da biblioteca foi perdido: %+v", loaded)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.GetSetting(ctx, "local.folders"); err != ErrSettingNotFound {
		t.Fatalf("esperava ErrSettingNotFound, obtive %v", err)
	}
	if err := db.SetSetting(ctx, "local.folders", `["/games"]`); err != nil {
		t.Fatal(err)
	}
	value, err := db.GetSetting(ctx, "local.folders")
	if err != nil || value != `["/games"]` {
		t.Fatalf("valor = %q, erro = %v", value, err)
	}
}

func TestStoreMetadataKeepsOriginalValuesSeparate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	game := &models.Game{
		GameID: "10", Source: "Steam", PluginID: "steam", Name: "Example",
		Description: "Original synopsis", Icon: "original icon",
	}
	if err := db.UpsertStoreGames(ctx, []*models.Game{game}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGameArtwork(ctx, game.ID, "icon", "chosen icon"); err != nil {
		t.Fatal(err)
	}
	newImport := &models.Game{
		GameID: "10", Source: "Steam", PluginID: "steam", Name: "Example",
		CoverImage: "official cover",
	}
	if err := db.UpsertStoreGames(ctx, []*models.Game{newImport}); err != nil {
		t.Fatal(err)
	}
	official, err := db.StoreMetadata(ctx, "steam", "10")
	if err != nil || official == nil || official.Description != "Original synopsis" ||
		official.Icon != "original icon" || official.CoverImage != "official cover" {
		t.Fatalf("snapshot original: %+v, %v", official, err)
	}
	visible, err := db.GetGame(ctx, game.ID)
	if err != nil || visible.Icon != "chosen icon" {
		t.Fatalf("jogo visível: %+v, %v", visible, err)
	}
}
