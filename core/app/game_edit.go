package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/playnux/core/models"
)

// GameEdit é o formulário completo da aba Geral das propriedades. Campos de
// lista chegam como arrays para não depender de um separador específico.
type GameEdit struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	ReleaseDate    string   `json:"release_date"`
	Version        string   `json:"version"`
	Genres         []string `json:"genres"`
	Developers     []string `json:"developers"`
	Publishers     []string `json:"publishers"`
	Platforms      []string `json:"platforms"`
	Tags           []string `json:"tags"`
	Categories     []string `json:"categories"`
	Features       []string `json:"features"`
	CriticScore    *int     `json:"critic_score"`
	CommunityScore *int     `json:"community_score"`
	UserScore      *int     `json:"user_score"`
	TrailerURL     string   `json:"trailer_url"`
}

func (c *App) SaveGameDetails(ctx context.Context, id string, edit GameEdit) (*models.Game, error) {
	game, err := c.db.GetGame(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.Join(strings.Fields(edit.Name), " ")
	if name == "" {
		return nil, fmt.Errorf("o nome do jogo não pode ficar vazio")
	}
	if utf8.RuneCountInString(name) > 300 {
		return nil, fmt.Errorf("o nome do jogo deve ter no máximo 300 caracteres")
	}
	description := strings.TrimSpace(edit.Description)
	if utf8.RuneCountInString(description) > 20000 {
		return nil, fmt.Errorf("a sinopse deve ter no máximo 20.000 caracteres")
	}
	version := strings.TrimSpace(edit.Version)
	if utf8.RuneCountInString(version) > 100 {
		return nil, fmt.Errorf("a versão deve ter no máximo 100 caracteres")
	}
	trailer := strings.TrimSpace(edit.TrailerURL)
	if trailer != "" {
		parsed, parseErr := url.Parse(trailer)
		if parseErr != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || len(trailer) > 500 {
			return nil, fmt.Errorf("informe um endereço de trailer http(s) válido")
		}
	}

	var release *time.Time
	if value := strings.TrimSpace(edit.ReleaseDate); value != "" {
		parsed, parseErr := time.Parse("2006-01-02", value)
		if parseErr != nil {
			return nil, fmt.Errorf("data de lançamento inválida; use AAAA-MM-DD")
		}
		release = &parsed
	}

	game.Name = name
	game.SortingName = name
	game.Description = description
	game.ReleaseDate = release
	game.ReleaseYear = 0
	if release != nil {
		game.ReleaseYear = release.Year()
	}
	game.Version = version
	game.TrailerURL = trailer
	game.Genres = cleanEditList(edit.Genres)
	game.Developers = cleanEditList(edit.Developers)
	game.Publishers = cleanEditList(edit.Publishers)
	game.Platforms = cleanEditList(edit.Platforms)
	game.Tags = cleanEditList(edit.Tags)
	game.Categories = cleanEditList(edit.Categories)
	game.Features = cleanEditList(edit.Features)
	game.CriticScore = cleanScore(edit.CriticScore)
	game.CommunityScore = cleanScore(edit.CommunityScore)
	game.UserScore = cleanScore(edit.UserScore)

	if err := c.db.UpdateGameDetails(ctx, game); err != nil {
		return nil, err
	}
	return c.db.GetGame(ctx, id)
}

func cleanEditList(values []string) []string {
	if len(values) > 50 {
		values = values[:50]
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" || utf8.RuneCountInString(value) > 200 {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func cleanScore(value *int) *int {
	if value == nil {
		return nil
	}
	score := *value
	if score < 0 || score > 100 {
		return nil
	}
	return &score
}
