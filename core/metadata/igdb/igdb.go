// Package igdb consulta a API do IGDB (Twitch) por sinopse, data, gêneros,
// empresas, nota da crítica e IDs de trailers do YouTube.
package igdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/playnux/core/metadata"
)

const (
	defaultAuthURL = "https://id.twitch.tv/oauth2/token"
	defaultAPIURL  = "https://api.igdb.com/v4/games"
	gameFields     = "name,summary,storyline,rating,game_modes.name,platforms.name,themes.name,keywords.name,first_release_date,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,aggregated_rating,videos.video_id"
)

// Client mantém o token de aplicação do Twitch em cache.
type Client struct {
	clientID     string
	clientSecret string
	http         *http.Client
	authURL      string
	apiURL       string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	nextRequest time.Time
}

func NewIGDB(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		http:         &http.Client{Timeout: 20 * time.Second},
		authURL:      defaultAuthURL,
		apiURL:       defaultAPIURL,
	}
}

type igdbGame struct {
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Storyline string   `json:"storyline"`
	Rating    *float64 `json:"rating"`
	GameModes []struct {
		Name string `json:"name"`
	} `json:"game_modes"`
	Platforms []struct {
		Name string `json:"name"`
	} `json:"platforms"`
	Themes []struct {
		Name string `json:"name"`
	} `json:"themes"`
	Keywords []struct {
		Name string `json:"name"`
	} `json:"keywords"`
	FirstReleaseDate int64    `json:"first_release_date"`
	AggregatedRating *float64 `json:"aggregated_rating"`
	Genres           []struct {
		Name string `json:"name"`
	} `json:"genres"`
	InvolvedCompanies []struct {
		Developer bool `json:"developer"`
		Publisher bool `json:"publisher"`
		Company   struct {
			Name string `json:"name"`
		} `json:"company"`
	} `json:"involved_companies"`
	Videos []struct {
		VideoID string `json:"video_id"`
	} `json:"videos"`
}

// Lookup busca pelo nome e devolve o primeiro resultado relevante do IGDB.
func (c *Client) Lookup(ctx context.Context, name string) (*metadata.Info, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("IGDB: nome vazio")
	}
	if c.clientID == "" || c.clientSecret == "" {
		return nil, fmt.Errorf("IGDB: credenciais não configuradas")
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`search %s; fields %s; limit 5;`, quote(name), gameFields)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", c.clientID)
	token, err := c.waitForRequest(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IGDB retornou status %d: %s", resp.StatusCode, snippet(body))
	}

	var games []igdbGame
	if err := json.Unmarshal(body, &games); err != nil {
		return nil, fmt.Errorf("IGDB: resposta inválida: %w", err)
	}
	if len(games) == 0 {
		return nil, metadata.ErrNotFound
	}
	return fromGame(bestGame(games, name)), nil
}

// bestGame evita que uma edição de outro console ou um resultado apenas
// parecido seja escolhido no lugar do jogo exato. Entre nomes iguais, a versão
// com PC/Windows/Linux tem prioridade.
func bestGame(games []igdbGame, name string) igdbGame {
	wanted := normalizeTitle(name)
	candidates := make([]igdbGame, 0, len(games))
	for _, game := range games {
		if normalizeTitle(game.Name) == wanted {
			candidates = append(candidates, game)
		}
	}
	if len(candidates) == 0 {
		candidates = games
	}
	for _, game := range candidates {
		if hasPCPlatform(game) {
			return game
		}
	}
	return candidates[0]
}

func normalizeTitle(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasPCPlatform(game igdbGame) bool {
	for _, platform := range game.Platforms {
		lower := strings.ToLower(platform.Name)
		if strings.Contains(lower, "pc") || strings.Contains(lower, "windows") || strings.Contains(lower, "linux") {
			return true
		}
	}
	return false
}

func (c *Client) waitForRequest(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if delay := time.Until(c.nextRequest); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.nextRequest = time.Now().Add(260 * time.Millisecond)
	return c.token, nil
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry.Add(-time.Minute)) {
		return nil
	}

	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("IGDB oauth: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IGDB oauth retornou status %d: %s", resp.StatusCode, snippet(body))
	}

	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return fmt.Errorf("IGDB oauth: resposta inválida: %w", err)
	}
	if token.AccessToken == "" {
		return fmt.Errorf("IGDB oauth: resposta sem access_token")
	}
	expires := time.Duration(token.ExpiresIn) * time.Second
	if expires <= 0 {
		expires = time.Hour
	}
	c.token = token.AccessToken
	c.tokenExpiry = time.Now().Add(expires)
	return nil
}

func fromGame(game igdbGame) *metadata.Info {
	info := &metadata.Info{
		Description: strings.TrimSpace(game.Summary),
	}
	if info.Description == "" {
		info.Description = strings.TrimSpace(game.Storyline)
	}
	for _, mode := range game.GameModes {
		if name := strings.TrimSpace(mode.Name); name != "" {
			info.Features = append(info.Features, name)
		}
	}
	seenTags := make(map[string]bool)
	for _, tags := range [][]struct {
		Name string `json:"name"`
	}{game.Themes, game.Keywords} {
		for _, tag := range tags {
			name := strings.TrimSpace(tag.Name)
			if key := strings.ToLower(name); name != "" && !seenTags[key] {
				info.Tags = append(info.Tags, name)
				seenTags[key] = true
			}
		}
	}
	for _, platform := range game.Platforms {
		if name := strings.TrimSpace(platform.Name); name != "" {
			info.Platforms = append(info.Platforms, name)
		}
	}
	if game.FirstReleaseDate > 0 {
		release := time.Unix(game.FirstReleaseDate, 0).UTC()
		info.ReleaseDate = &release
	}
	for _, genre := range game.Genres {
		if genre.Name != "" {
			info.Genres = append(info.Genres, genre.Name)
		}
	}
	for _, company := range game.InvolvedCompanies {
		name := strings.TrimSpace(company.Company.Name)
		if name == "" {
			continue
		}
		if company.Developer {
			info.Developers = append(info.Developers, name)
		}
		if company.Publisher {
			info.Publishers = append(info.Publishers, name)
		}
	}
	info.CriticScore = ratingScore(game.AggregatedRating)
	info.CommunityScore = ratingScore(game.Rating)
	for _, video := range game.Videos {
		if id := strings.TrimSpace(video.VideoID); id != "" {
			info.TrailerURL = "https://www.youtube.com/watch?v=" + url.QueryEscape(id)
			break
		}
	}
	return info
}

func ratingScore(value *float64) *int {
	if value == nil {
		return nil
	}
	score := min(100, max(0, int(math.Round(*value))))
	return &score
}

// quote escapa o termo para a linguagem de consulta do IGDB. A API não aceita
// parâmetros SQL; o termo vai entre aspas e não pode conter quebras de linha.
func quote(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	return strconv.Quote(value)
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}
