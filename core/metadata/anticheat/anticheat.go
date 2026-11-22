// Package anticheat looks up Linux/Wine compatibility information from the
// AreWeAntiCheatYet community catalog.
package anticheat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	catalogURL = "https://raw.githubusercontent.com/AreWeAntiCheatYet/AreWeAntiCheatYet/master/games.json"
	cacheTTL   = 6 * time.Hour
) // Entry is one game in the AreWeAntiCheatYet catalog.
type Entry struct {
	Name        string             `json:"name"`
	URL         string             `json:"url"`
	Status      string             `json:"status"`
	AntiCheats  []string           `json:"anticheats"`
	Notes       [][]string         `json:"notes"`
	StoreIDs    map[string]StoreID `json:"storeIds"`
	Slug        string             `json:"slug"`
	DateChanged string             `json:"dateChanged"`
}

// StoreID accepts the catalog's mixed formats: a plain string or an object
// such as Epic's {"namespace": "...", "slug": "..."}.
type StoreID []string

func (s *StoreID) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err == nil {
		*s = many
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return nil
	}
	values := make([]string, 0, 3)
	for _, key := range []string{"id", "appId", "productId", "namespace", "slug"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	*s = values
	return nil
}

// Info is the result shown as a warning or info banner.
type Info struct {
	Found      bool     `json:"found"`
	Name       string   `json:"name,omitempty"`
	Status     string   `json:"status,omitempty"`
	Risk       string   `json:"risk,omitempty"` // safe, caution, warning, danger
	Warning    bool     `json:"warning"`
	AntiCheats []string `json:"anticheats,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	URL        string   `json:"url,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Client downloads and caches the anti-cheat catalog.
type Client struct {
	CatalogURL string
	HTTP       *http.Client

	mu      sync.Mutex
	entries []Entry
	expires time.Time
	loading sync.Mutex
}

func NewClient() *Client {
	return &Client{
		CatalogURL: catalogURL,
		HTTP:       &http.Client{Timeout: 20 * time.Second},
	}
}

// Lookup finds a game by store ID, falling back to a normalized title match.
func (c *Client) Lookup(ctx context.Context, name, pluginID, gameID string) (Info, error) {
	if c == nil {
		return Info{}, fmt.Errorf("anticheat: client is not configured")
	}
	entries, err := c.catalog(ctx)
	if err != nil {
		return Info{}, err
	}
	pluginID = strings.ToLower(strings.TrimSpace(pluginID))
	gameID = strings.TrimSpace(gameID)

	for _, entry := range entries {
		if gameID != "" && pluginID != "" {
			for store, ids := range entry.StoreIDs {
				if !strings.EqualFold(store, pluginID) {
					continue
				}
				for _, id := range ids {
					if strings.EqualFold(strings.TrimSpace(id), gameID) {
						return buildInfo(entry), nil
					}
				}
			}
		}
	}
	normalized := normalizeTitle(name)
	if normalized == "" {
		return Info{}, nil
	}
	for _, entry := range entries {
		if normalizeTitle(entry.Name) == normalized {
			return buildInfo(entry), nil
		}
	}
	return Info{}, nil
}

func (c *Client) catalog(ctx context.Context) ([]Entry, error) {
	c.mu.Lock()
	if len(c.entries) > 0 && time.Now().Before(c.expires) {
		entries := c.entries
		c.mu.Unlock()
		return entries, nil
	}
	c.mu.Unlock()

	// Avoid downloading the large catalog twice when several details panels
	// are opened at nearly the same time.
	c.loading.Lock()
	defer c.loading.Unlock()
	c.mu.Lock()
	if len(c.entries) > 0 && time.Now().Before(c.expires) {
		entries := c.entries
		c.mu.Unlock()
		return entries, nil
	}
	c.mu.Unlock()

	endpoint := c.CatalogURL
	if endpoint == "" {
		endpoint = catalogURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Playnux/1.0 (+https://github.com/playnux)")
	request.Header.Set("Accept", "application/json")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("anticheat: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anticheat: catalog returned status %d", response.StatusCode)
	}
	var entries []Entry
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("anticheat: decode catalog: %w", err)
	}

	c.mu.Lock()
	c.entries = entries
	c.expires = time.Now().Add(cacheTTL)
	c.mu.Unlock()
	return entries, nil
}

func buildInfo(entry Entry) Info {
	status := strings.TrimSpace(entry.Status)
	info := Info{
		Found:      true,
		Name:       entry.Name,
		Status:     status,
		AntiCheats: append([]string(nil), entry.AntiCheats...),
		URL:        "https://areweanticheatyet.com/?search=" + strings.ReplaceAll(entry.Slug, " ", "%20"),
	}
	if info.URL == "https://areweanticheatyet.com/?search=" {
		info.URL = "https://areweanticheatyet.com/"
	}
	for _, note := range entry.Notes {
		if len(note) > 0 && strings.TrimSpace(note[0]) != "" {
			info.Notes = append(info.Notes, strings.TrimSpace(note[0]))
		}
	}
	switch strings.ToLower(status) {
	case "denied":
		info.Warning = true
		info.Risk = "danger"
	case "broken":
		info.Warning = true
		info.Risk = "warning"
	case "running":
		info.Risk = "caution"
	case "supported":
		info.Risk = "safe"
	default:
		info.Risk = "caution"
	}
	return info
}

func normalizeTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("™", "", "®", "", "©", "").Replace(value)
	var builder strings.Builder
	space := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			space = false
		case char > 127:
			builder.WriteRune(char)
			space = false
		default:
			if !space && builder.Len() > 0 {
				builder.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}
