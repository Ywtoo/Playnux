package igdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLookupParsesGameVideosAndGenres(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método do token = %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "client" || r.Form.Get("client_secret") != "secret" {
			t.Errorf("credenciais = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-123",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/games", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Client-ID") != "client" || r.Header.Get("Authorization") != "Bearer token-123" {
			t.Errorf("headers = %v", r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `search "Hades"`) ||
			!strings.Contains(string(body), "involved_companies.company.name") ||
			strings.Contains(string(body), "developers.name") || strings.Contains(string(body), "publishers.name") {
			t.Errorf("consulta = %s", string(body))
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"name":               "Hades",
			"storyline":          "Rogue-like de ação.",
			"rating":             88.1,
			"game_modes":         []map[string]string{{"name": "Single player"}},
			"platforms":          []map[string]string{{"name": "PC"}},
			"first_release_date": int64(1603411200),
			"aggregated_rating":  92.4,
			"genres":             []map[string]string{{"name": "Role-playing (RPG)"}, {"name": "Adventure"}},
			"involved_companies": []map[string]any{{
				"company":   map[string]string{"name": "Supergiant Games"},
				"developer": true, "publisher": true,
			}},
			"videos": []map[string]string{{"video_id": "abc123"}},
		}})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewIGDB("client", "secret")
	client.authURL = server.URL + "/token"
	client.apiURL = server.URL + "/games"

	info, err := client.Lookup(context.Background(), "Hades")
	if err != nil {
		t.Fatal(err)
	}
	if info.Description != "Rogue-like de ação." || info.ReleaseDate == nil {
		t.Fatalf("info = %+v", info)
	}
	if info.CriticScore == nil || *info.CriticScore != 92 {
		t.Fatalf("nota = %v", info.CriticScore)
	}
	if len(info.Genres) != 2 || info.Genres[0] != "Role-playing (RPG)" {
		t.Fatalf("gêneros = %v", info.Genres)
	}
	if len(info.Developers) != 1 || info.Developers[0] != "Supergiant Games" {
		t.Fatalf("devs = %v", info.Developers)
	}
	if len(info.Publishers) != 1 || info.Publishers[0] != "Supergiant Games" {
		t.Fatalf("publishers = %v", info.Publishers)
	}
	if info.CommunityScore == nil || *info.CommunityScore != 88 ||
		len(info.Features) != 1 || len(info.Platforms) != 1 {
		t.Fatalf("ficha técnica = %+v", info)
	}
	if info.TrailerURL != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("trailer = %q", info.TrailerURL)
	}
}

func TestLookupRejectsMissingCredentials(t *testing.T) {
	client := NewIGDB("", "")
	if _, err := client.Lookup(context.Background(), "Hades"); err == nil {
		t.Fatal("esperava erro sem credenciais")
	}
}

func TestQuoteEscapesSearchTerm(t *testing.T) {
	got := quote("A \"Game\"\nName")
	if strings.Contains(got, "\n") || !strings.HasPrefix(got, `"`) {
		t.Fatalf("quote = %q", got)
	}
}

func TestBestGamePrefersExactNameAndPCPlatform(t *testing.T) {
	games := []igdbGame{
		{Name: "Hollow Knight: Silksong", Platforms: []struct {
			Name string `json:"name"`
		}{{Name: "PC (Microsoft Windows)"}}},
		{Name: "Hollow Knight", Platforms: []struct {
			Name string `json:"name"`
		}{{Name: "PlayStation Vita"}}},
		{Name: "Hollow Knight", Platforms: []struct {
			Name string `json:"name"`
		}{{Name: "PC (Microsoft Windows)"}}},
	}
	got := bestGame(games, "Hollow Knight")
	if !hasPCPlatform(got) || got.Name != "Hollow Knight" {
		t.Fatalf("melhor jogo = %+v", got)
	}
}
