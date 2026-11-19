package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
)

func main() {
	dbPath := "/tmp/playnux_test.db"
	_ = os.Remove(dbPath)

	fmt.Println("=== TESTE DO CORE: BANCO COMPLETO + BIBLIOTECA DIGITAL ===")
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Erro ao abrir banco: %v", err)
	}
	defer db.Close()
	defer os.Remove(dbPath)

	// Simula a sincronização da conta digital (jogos comprados na nuvem e instalados no disco)
	now := time.Now()
	sampleGames := []*models.Game{
		{
			GameID:           "1091500",
			PluginID:         "steam",
			Source:           "Steam",
			Name:             "Cyberpunk 2077",
			IsInstalled:      true,
			InstallDirectory: "/home/gabri/.steam/steam/steamapps/common/Cyberpunk 2077",
			CoverImage:       "https://steamcdn-a.akamaihd.net/steam/apps/1091500/library_600x900.jpg",
			Genres:           []string{"RPG", "Open World", "Sci-Fi"},
			Developers:       []string{"CD PROJEKT RED"},
			Publishers:       []string{"CD PROJEKT RED"},
			Playtime:         154200,
			LastActivity:     &now,
		},
		{
			GameID:      "1207658695",
			PluginID:    "gog",
			Source:      "GOG",
			Name:        "The Witcher 3: Wild Hunt",
			IsInstalled: false, // Comprado na conta, mas NÃO está baixado no PC
			CoverImage:  "https://images.gog.com/witcher3.jpg",
			Genres:      []string{"RPG", "Fantasia"},
			Developers:  []string{"CD PROJEKT RED"},
			Publishers:  []string{"CD PROJEKT RED"},
			Playtime:    7200,
		},
		{
			GameID:      "epic_death_stranding",
			PluginID:    "epic",
			Source:      "Epic",
			Name:        "Death Stranding",
			IsInstalled: false, // Resgatado na conta Epic, disponível para download
			CoverImage:  "https://epicgames.com/deathstranding.jpg",
			Genres:      []string{"Ação", "Aventura"},
			Developers:  []string{"Kojima Productions"},
			Publishers:  []string{"505 Games"},
		},
	}

	ctx := context.Background()

	// 1. Inserção em lote (Upsert)
	start := time.Now()
	if err := db.UpsertGames(ctx, sampleGames); err != nil {
		log.Fatalf("Erro ao fazer upsert: %v", err)
	}
	fmt.Printf("✓ 3 jogos de lojas diferentes (Steam, GOG, Epic) gravados com tabela completa em: %v\n\n", time.Since(start))

	// 2. Busca instantânea FTS5 (busca rápida por texto)
	term := "witcher"
	res, err := db.SearchGames(ctx, term, 10)
	if err != nil {
		log.Fatalf("Erro na busca: %v", err)
	}

	fmt.Printf("🔍 Busca FTS5 por '%s' retornou %d jogo(s):\n", term, len(res))
	for _, g := range res {
		status := "Disponível na Nuvem (Não instalado)"
		if g.IsInstalled {
			status = "Instalado no Disco"
		}
		fmt.Printf("  • [%s] %s | Loja: %s | Status: %s\n", g.Source, g.Name, g.Source, status)
	}

	fmt.Println("\n✓ SUCESSO: O banco já nasceu completo com suporte a biblioteca digital de todas as lojas!")
}
