package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/store/builtins/linux"
)

func TestSaveGameDetailsNormalizesAndClearsFields(t *testing.T) {
	ctx := context.Background()
	core, err := NewWithProviders(":memory:", nil, nil, linux.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()

	original := &models.Game{
		GameID: "local", Source: "Manual", PluginID: "linux", Name: "Nome antigo",
		Description: "descrição antiga", Platforms: []string{"PlayStation Vita"},
	}
	if err := core.db.UpsertGames(ctx, []*models.Game{original}); err != nil {
		t.Fatal(err)
	}
	critic := 92
	updated, err := core.SaveGameDetails(ctx, original.ID, GameEdit{
		Name: "  Hollow   Knight  ", Description: "  sinopse nova  ", ReleaseDate: "2017-02-24",
		Genres: []string{"Ação", " ação ", ""}, Platforms: []string{"PC (Windows)", "Linux"},
		Tags: []string{"Metroidvania"}, CriticScore: &critic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Hollow Knight" || updated.SortingName != "Hollow Knight" {
		t.Fatalf("nome = %q / %q", updated.Name, updated.SortingName)
	}
	if updated.ReleaseYear != 2017 || updated.Description != "sinopse nova" {
		t.Fatalf("metadados = %+v", updated)
	}
	if len(updated.Genres) != 1 || updated.Genres[0] != "Ação" {
		t.Fatalf("gêneros = %v", updated.Genres)
	}
	if len(updated.Platforms) != 2 || updated.Platforms[0] != "PC (Windows)" {
		t.Fatalf("plataformas = %v", updated.Platforms)
	}
	if updated.CriticScore == nil || *updated.CriticScore != 92 {
		t.Fatalf("nota = %v", updated.CriticScore)
	}

	if _, err := core.SaveGameDetails(ctx, original.ID, GameEdit{Name: ""}); err == nil {
		t.Fatal("esperava erro para nome vazio")
	}
}

func TestApplyCompatibilityBuildsProtonCommand(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Hollow Knight.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	proton := filepath.Join(root, "GE-Proton10-18", "proton")
	if err := os.MkdirAll(filepath.Dir(proton), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proton, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	core, err := NewWithProviders(filepath.Join(root, "playnux.db"), nil, nil, linux.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()

	game := &models.Game{
		GameID: "manual:" + exe, PluginID: "linux", Source: "Manual", Name: "Hollow Knight",
		Executable: "/usr/bin/wine", Arguments: []string{exe},
	}
	options := launcher.Options{Runner: "proton", ProtonPath: proton}
	if err := core.applyCompatibility(game, &options); err != nil {
		t.Fatal(err)
	}
	if options.Command != proton || len(options.CommandArgs) != 2 || options.CommandArgs[0] != "run" || options.CommandArgs[1] != exe {
		t.Fatalf("comando Proton = %q %v", options.Command, options.CommandArgs)
	}
	if options.Environment["STEAM_COMPAT_DATA_PATH"] == "" {
		t.Fatal("prefixo Proton não configurado")
	}
}
