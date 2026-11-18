// Package steamgriddb busca capas verticais 600x900, banners hero e logos
// transparentes no SteamGridDB. A API exige uma chave pessoal.
package steamgriddb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/playnux/core/metadata"
)

const defaultBaseURL = "https://www.steamgriddb.com/api/v2"

type Client struct {
	apiKey string
	http   *http.Client
	base   string
}

func NewSteamGridDB(apiKey string) *Client {
	return &Client{
		apiKey: strings.TrimSpace(apiKey),
		http:   &http.Client{Timeout: 20 * time.Second},
		base:   defaultBaseURL,
	}
}

type searchEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ImageChoice struct {
	URL    string `json:"url"`
	Thumb  string `json:"thumb"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Score  int    `json:"score"`
}

func (c *Client) findGame(ctx context.Context, name string) (searchEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return searchEntry{}, fmt.Errorf("SteamGridDB: nome vazio")
	}
	if c.apiKey == "" {
		return searchEntry{}, fmt.Errorf("SteamGridDB: chave não configurada")
	}
	var found []searchEntry
	if err := c.get(ctx, "/search/autocomplete/"+url.PathEscape(name), &found); err != nil {
		return searchEntry{}, err
	}
	if len(found) == 0 {
		return searchEntry{}, metadata.ErrNotFound
	}
	return bestMatch(found, name), nil
}

type imageEntry = ImageChoice

var imageEndpoints = map[string]string{
	"cover":      "/grids/game/%d?dimensions=600x900&types=static",
	"background": "/heroes/game/%d?types=static",
	"logo":       "/logos/game/%d?types=static",
	"icon":       "/icons/game/%d?mimes=image/png",
}

// SearchImages returns a bounded page of artwork for the visual picker.
func (c *Client) SearchImages(ctx context.Context, name, field string) ([]ImageChoice, error) {
	endpoint, ok := imageEndpoints[field]
	if !ok {
		return nil, fmt.Errorf("campo de imagem desconhecido")
	}
	game, err := c.findGame(ctx, name)
	if err != nil {
		return nil, err
	}
	var images []ImageChoice
	if err := c.get(ctx, fmt.Sprintf(endpoint, game.ID), &images); err != nil {
		return nil, err
	}
	sort.SliceStable(images, func(i, j int) bool { return images[i].Score > images[j].Score })
	return images, nil
}

// LookupImage fetches just the artwork selected in the editor.
func (c *Client) LookupImage(ctx context.Context, name, field string) (string, error) {
	endpoint, ok := imageEndpoints[field]
	if !ok {
		return "", fmt.Errorf("campo de imagem desconhecido")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	game, err := c.findGame(ctx, name)
	if err != nil {
		return "", err
	}
	var images []imageEntry
	if err := c.getImages(ctx, fmt.Sprintf(endpoint, game.ID), &images); err != nil {
		return "", err
	}
	best := highestScore(images)
	if field == "cover" {
		best = bestGrid(images)
	}
	if best == "" {
		return "", fmt.Errorf("nenhuma imagem desse tipo no SteamGridDB")
	}
	return best, nil
}

// Lookup finds a game and returns its best assets.
func (c *Client) Lookup(ctx context.Context, name string) (*metadata.Artwork, error) {
	game, err := c.findGame(ctx, name)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	var grids, heroes, logos, icons []imageEntry
	var failures []error
	for _, asset := range []struct {
		path   string
		target *[]imageEntry
	}{
		{fmt.Sprintf(imageEndpoints["cover"], game.ID), &grids},
		{fmt.Sprintf(imageEndpoints["background"], game.ID), &heroes},
		{fmt.Sprintf(imageEndpoints["logo"], game.ID), &logos},
		{fmt.Sprintf(imageEndpoints["icon"], game.ID), &icons},
	} {
		if err := c.getImages(ctx, asset.path, asset.target); err != nil {
			failures = append(failures, err)
		}
	}

	art := &metadata.Artwork{
		Cover: bestGrid(grids), Background: highestScore(heroes),
		Logo: highestScore(logos), Icon: highestScore(icons),
	}
	if *art == (metadata.Artwork{}) {
		if len(failures) > 0 {
			return nil, errors.Join(failures...)
		}
		return nil, metadata.ErrNotFound
	}
	return art, nil
}

type pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

func (c *Client) getImages(ctx context.Context, path string, images *[]imageEntry) error {
	endpoint, err := url.Parse(path)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	for page := 0; ; page++ {
		query.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()
		var batch []imageEntry
		var paging pagination
		if err := c.getPage(ctx, endpoint.String(), &batch, &paging); err != nil {
			return err
		}
		*images = append(*images, batch...)
		if len(batch) == 0 || paging.Page != page || paging.Limit <= 0 || (page+1) >= (paging.Total-1)/paging.Limit+1 {
			return nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.getPage(ctx, path, out, nil)
}

func (c *Client) getPage(ctx context.Context, path string, out any, paging *pagination) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("SteamGridDB: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SteamGridDB retornou status %d: %s", resp.StatusCode, snippet(body))
	}

	var envelope struct {
		pagination
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("SteamGridDB: resposta inválida: %w", err)
	}
	if !envelope.Success {
		return fmt.Errorf("SteamGridDB: requisição recusada")
	}
	if paging != nil {
		*paging = envelope.pagination
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("SteamGridDB: dados inválidos: %w", err)
	}
	return nil
}

func bestMatch(entries []searchEntry, name string) searchEntry {
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.Name), name) {
			return entry
		}
	}
	return entries[0]
}

func bestGrid(grids []imageEntry) string {
	portrait := make([]imageEntry, 0, len(grids))
	for _, grid := range grids {
		if grid.Width == 600 && grid.Height == 900 {
			portrait = append(portrait, grid)
		}
	}
	if best := highestScore(portrait); best != "" {
		return best
	}
	return highestScore(grids)
}

// highestScore uses the numeric score instead of the API's result order.
func highestScore(images []imageEntry) string {
	var best *imageEntry
	for i := range images {
		image := &images[i]
		if image.URL != "" && (best == nil || image.Score > best.Score) {
			best = image
		}
	}
	if best == nil {
		return ""
	}
	return best.URL
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}
