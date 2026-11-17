package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

type fakeProvider struct {
	id           string
	name         string
	spec         store.WebLoginSpec
	games        []*models.Game
	auth         store.StoreAuth
	exchangeAuth store.StoreAuth
	exchangeCode string
	cookieAuth   store.StoreAuth
	seenCookies  map[string]string
}

func (f *fakeProvider) ID() string   { return f.id }
func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Authenticate(_ context.Context, auth store.StoreAuth) error {
	f.auth = auth
	return nil
}

func (f *fakeProvider) IsAuthenticated() bool {
	return f.auth.AccessToken != "" || f.auth.AccountID != ""
}

func (f *fakeProvider) FetchLibrary(context.Context) ([]*models.Game, error) {
	return f.games, nil
}

func (f *fakeProvider) LaunchCommand(*models.Game) (string, []string, error) {
	return "", nil, nil
}

func (f *fakeProvider) UninstallCommand(*models.Game) (string, []string, error) {
	return "", nil, nil
}

func (f *fakeProvider) WebLogin() store.WebLoginSpec { return f.spec }

type cookieProvider struct{ *fakeProvider }

func (p *cookieProvider) AuthFromCookies(cookies map[string]string) (store.StoreAuth, error) {
	p.seenCookies = cookies
	return p.cookieAuth, nil
}

type codeProvider struct{ *fakeProvider }

func (p *codeProvider) ExchangeCode(_ context.Context, code string) (store.StoreAuth, error) {
	p.exchangeCode = code
	return p.exchangeAuth, nil
}

type fakeView struct {
	req    LoginRequest
	result LoginResult
	err    error
}

func (v *fakeView) OpenLogin(_ context.Context, req LoginRequest) (LoginResult, error) {
	v.req = req
	return v.result, v.err
}

func newTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "playnux.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLoginWithCookieSessionPersistsAndSyncs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	provider := &cookieProvider{&fakeProvider{
		id:   "steam",
		name: "Steam",
		spec: store.WebLoginSpec{
			URL:               "https://steamcommunity.com/login/home/?goto=",
			CookieNames:       []string{"steamLoginSecure"},
			RedirectURLPrefix: "https://steamcommunity.com/",
		},
		cookieAuth: store.StoreAuth{
			AccountID: "76561198000000000",
			Cookies:   "steamLoginSecure=76561198000000000%7C%7Ctoken",
		},
		games: []*models.Game{{
			GameID:    "1091500",
			PluginID:  "steam",
			Source:    "Steam",
			Name:      "Cyberpunk 2077",
			Platforms: []string{"PC (Linux)"},
		}},
	}}
	view := &fakeView{result: LoginResult{Cookies: map[string]string{
		"steamLoginSecure": "76561198000000000%7C%7Ctoken",
	}}}

	manager := NewManager(db, view, provider)
	session, games, err := manager.Login(ctx, "steam")
	if err != nil {
		t.Fatal(err)
	}

	if view.req.PluginID != "steam" || view.req.URL != provider.spec.URL {
		t.Fatalf("request enviado à WebView incorreto: %+v", view.req)
	}
	if provider.seenCookies["steamLoginSecure"] == "" {
		t.Fatal("cookies não chegaram ao provedor")
	}
	if session.AccountID != "76561198000000000" {
		t.Fatalf("account id = %q", session.AccountID)
	}
	if provider.auth.AccountID != session.AccountID {
		t.Fatalf("Authenticate não recebeu a sessão: %+v", provider.auth)
	}
	if len(games) != 1 {
		t.Fatalf("esperava 1 jogo sincronizado, obtive %d", len(games))
	}

	loaded, err := db.LoadSession(ctx, "steam")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Cookies != session.Cookies || loaded.UpdatedAt.IsZero() {
		t.Fatalf("sessão persistida incorreta: %+v", loaded)
	}

	found, err := db.SearchGames(ctx, "cyberpunk", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "Cyberpunk 2077" {
		t.Fatalf("biblioteca não foi gravada no SQLite: %+v", found)
	}
}

func TestLoginWithCodeExchange(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	provider := &codeProvider{&fakeProvider{
		id:   "epic",
		name: "Epic Games Store",
		spec: store.WebLoginSpec{
			URL:               "https://www.epicgames.com/id/login",
			CookieNames:       []string{"EPIC_BEARER_TOKEN"},
			RedirectURLPrefix: "https://www.epicgames.com/id/api/redirect",
		},
		exchangeAuth: store.StoreAuth{
			AccessToken:  "epic-token",
			RefreshToken: "epic-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
		},
		games: []*models.Game{{
			GameID:    "Sugar",
			PluginID:  "epic",
			Source:    "Epic",
			Name:      "Sugar",
			Platforms: []string{"PC (Windows)"},
		}},
	}}
	view := &fakeView{result: LoginResult{
		RedirectURL: "https://www.epicgames.com/id/api/redirect?code=epic%2Bcode",
		Cookies:     map[string]string{"EPIC_BEARER_TOKEN": "cookie-token"},
	}}

	manager := NewManager(db, view, provider)
	_, games, err := manager.Login(ctx, "epic")
	if err != nil {
		t.Fatal(err)
	}
	if provider.exchangeCode != "epic+code" {
		t.Fatalf("código trocado = %q", provider.exchangeCode)
	}
	if provider.auth.AccessToken != "epic-token" || provider.auth.RefreshToken != "epic-refresh" {
		t.Fatalf("tokens não aplicados ao provedor: %+v", provider.auth)
	}
	if provider.auth.Cookies == "" {
		t.Fatal("cookies da WebView deveriam ser preservados na sessão")
	}
	if len(games) != 1 {
		t.Fatalf("esperava 1 jogo, obtive %d", len(games))
	}

	loaded, err := db.LoadSession(ctx, "epic")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "epic-token" || loaded.ExpiresAt.IsZero() {
		t.Fatalf("sessão Epic persistida incorreta: %+v", loaded)
	}
}

func TestExtractAuthorizationCode(t *testing.T) {
	cases := []struct {
		name   string
		result LoginResult
		want   string
	}{
		{
			name:   "redirect query",
			result: LoginResult{RedirectURL: "https://embed.gog.com/on_login_success?origin=client&code=abc"},
			want:   "abc",
		},
		{
			name:   "epic json body",
			result: LoginResult{Body: `{"authorizationCode":"body-code","redirectUrl":"x"}`},
			want:   "body-code",
		},
		{
			name:   "cookie fallback",
			result: LoginResult{Cookies: map[string]string{"exchange_code": "cookie-code"}},
			want:   "cookie-code",
		},
		{
			name:   "no credentials",
			result: LoginResult{RedirectURL: "https://embed.gog.com/on_login_success"},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractAuthorizationCode(tc.result); got != tc.want {
				t.Fatalf("code = %q, esperava %q", got, tc.want)
			}
		})
	}
}

func TestLoginWebViewErrorSavesNothing(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	provider := &cookieProvider{&fakeProvider{
		id:   "steam",
		name: "Steam",
		spec: store.WebLoginSpec{URL: "https://steamcommunity.com/login"},
	}}
	manager := NewManager(db, &fakeView{err: errors.New("janela fechada")}, provider)

	if _, _, err := manager.Login(ctx, "steam"); err == nil {
		t.Fatal("esperava erro quando a WebView falha")
	}
	if _, err := db.LoadSession(ctx, "steam"); !errors.Is(err, database.ErrSessionNotFound) {
		t.Fatalf("nenhuma sessão deveria ter sido salva, obtive %v", err)
	}
}
