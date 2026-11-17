// Package auth orquestra o login por WebView das lojas (estilo Playnite).
//
// A camada Wails implementa WebView: abre a URL da loja, escuta os cookies e o
// redirect de sucesso, fecha a janela e devolve o resultado. O Core troca o
// código de autorização por tokens (Epic/GOG) ou extrai a sessão dos cookies
// (Steam), cifra a sessão no SQLite e dispara a sincronização da biblioteca.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

// LoginRequest é passado à ponte WebView para abrir a janela de login.
type LoginRequest struct {
	PluginID          string
	URL               string
	CookieNames       []string
	RedirectURLPrefix string
}

// LoginResult é devolvido pela ponte quando o usuário termina o login (ou a
// janela é fechada). Cookies mapeia nome -> valor.
type LoginResult struct {
	RedirectURL string
	Body        string
	Cookies     map[string]string
}

// WebView é o contrato mínimo que a camada de UI (Wails) implementa.
type WebView interface {
	OpenLogin(ctx context.Context, req LoginRequest) (LoginResult, error)
}

// Manager liga os provedores de loja ao banco e à WebView.
type Manager struct {
	db        *database.DB
	view      WebView
	providers map[string]store.StoreProvider
}

func NewManager(db *database.DB, view WebView, providers ...store.StoreProvider) *Manager {
	byID := make(map[string]store.StoreProvider, len(providers))
	for _, provider := range providers {
		byID[provider.ID()] = provider
	}
	return &Manager{db: db, view: view, providers: byID}
}

// Login abre a janela oficial da loja, persiste a sessão cifrada e sincroniza
// toda a biblioteca daquela loja.
func (m *Manager) Login(ctx context.Context, pluginID string) (*models.Session, []*models.Game, error) {
	provider, ok := m.providers[pluginID]
	if !ok {
		return nil, nil, fmt.Errorf("loja %q não registrada", pluginID)
	}
	if m.view == nil {
		return nil, nil, fmt.Errorf("login de %s: WebView não conectada à UI", provider.Name())
	}
	web, ok := provider.(store.WebAuthenticator)
	if !ok {
		return nil, nil, fmt.Errorf("loja %s não suporta login por WebView", provider.Name())
	}
	spec := web.WebLogin()
	if spec.URL == "" {
		return nil, nil, fmt.Errorf("loja %s não definiu a URL de login", provider.Name())
	}

	result, err := m.view.OpenLogin(ctx, LoginRequest{
		PluginID:          pluginID,
		URL:               spec.URL,
		CookieNames:       spec.CookieNames,
		RedirectURLPrefix: spec.RedirectURLPrefix,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("login de %s falhou: %w", provider.Name(), err)
	}

	auth, err := credentials(ctx, provider, result)
	if err != nil {
		return nil, nil, err
	}
	if err := provider.Authenticate(ctx, auth); err != nil {
		return nil, nil, fmt.Errorf("credenciais de %s inválidas: %w", provider.Name(), err)
	}

	session := store.SessionFromAuth(pluginID, auth)
	if err := m.db.SaveSession(ctx, session); err != nil {
		return nil, nil, err
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		return session, nil, fmt.Errorf("sessão salva, mas a sincronização de %s falhou: %w",
			provider.Name(), err)
	}
	if err := m.db.UpsertStoreGames(ctx, games); err != nil {
		return session, nil, err
	}

	if refreshed, err := m.persistRefreshed(ctx, pluginID, provider); err == nil && refreshed != nil {
		session = refreshed
	}
	return session, games, nil
}

// Sync reaproveita a sessão salva, renova o token se necessário e atualiza a
// biblioteca (usado na inicialização do app e no botão "sincronizar").
func (m *Manager) Sync(ctx context.Context, pluginID string) ([]*models.Game, error) {
	provider, ok := m.providers[pluginID]
	if !ok {
		return nil, fmt.Errorf("loja %q não registrada", pluginID)
	}

	if local, ok := provider.(store.LocalProvider); ok && local.IsLocal() {
		if err := provider.Authenticate(ctx, store.StoreAuth{}); err != nil {
			return nil, fmt.Errorf("preparar %s: %w", provider.Name(), err)
		}
	} else {
		session, err := m.db.LoadSession(ctx, pluginID)
		if errors.Is(err, database.ErrSessionNotFound) {
			return nil, fmt.Errorf("loja %s não autenticada", provider.Name())
		}
		if err != nil {
			return nil, err
		}
		if err := provider.Authenticate(ctx, store.AuthFromSession(session)); err != nil {
			return nil, fmt.Errorf("sessão de %s inválida: %w", provider.Name(), err)
		}
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.db.UpsertStoreGames(ctx, games); err != nil {
		return nil, err
	}

	// Uma falha ao persistir tokens renovados não invalida a sincronização.
	_, _ = m.persistRefreshed(ctx, pluginID, provider)
	return games, nil
}

// ImportInstalled lê somente os jogos instalados localmente, sem abrir WebView
// nem exigir sessão digital. É o caminho de emergência quando o login web não
// captura cookies (Steam/WebKitGTK) ou o usuário não quer entrar agora.
func (m *Manager) ImportInstalled(ctx context.Context, pluginID string) ([]*models.Game, error) {
	provider, ok := m.providers[pluginID]
	if !ok {
		return nil, fmt.Errorf("loja %q não registrada", pluginID)
	}
	importer, ok := provider.(store.InstalledImporter)
	if !ok {
		return nil, fmt.Errorf("loja %s não suporta importação apenas de instalados", provider.Name())
	}
	games, err := importer.FetchInstalled(ctx)
	if err != nil {
		return nil, fmt.Errorf("importar instalados de %s: %w", provider.Name(), err)
	}
	if err := m.db.UpsertStoreGames(ctx, games); err != nil {
		return nil, err
	}
	return games, nil
}

// ConnectAccount conecta uma loja pelo identificador público da conta, sem
// senha. Hoje é usado pelo SteamID64 e mantém o scanner local funcional.
func (m *Manager) ConnectAccount(ctx context.Context, pluginID, accountID string) ([]*models.Game, error) {
	provider, ok := m.providers[pluginID]
	if !ok {
		return nil, fmt.Errorf("loja %q não registrada", pluginID)
	}
	connector, ok := provider.(store.AccountIDAuthenticator)
	if !ok {
		return nil, fmt.Errorf("loja %s não aceita conexão por ID de conta", provider.Name())
	}
	if err := connector.AuthenticateAccountID(ctx, accountID); err != nil {
		return nil, err
	}

	auth := store.StoreAuth{AccountID: strings.TrimSpace(accountID)}
	if snapshotter, ok := provider.(store.AuthSnapshotter); ok {
		auth = snapshotter.AuthSnapshot()
	}
	if err := m.db.SaveSession(ctx, store.SessionFromAuth(pluginID, auth)); err != nil {
		return nil, err
	}

	games, err := provider.FetchLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("conta conectada, mas a importação falhou: %w", err)
	}
	if err := m.db.UpsertStoreGames(ctx, games); err != nil {
		return nil, err
	}
	return games, nil
}

// persistRefreshed grava tokens renovados durante a sincronização.
func (m *Manager) persistRefreshed(ctx context.Context, pluginID string, provider store.StoreProvider) (*models.Session, error) {
	snapshotter, ok := provider.(store.AuthSnapshotter)
	if !ok {
		return nil, nil
	}
	session := store.SessionFromAuth(pluginID, snapshotter.AuthSnapshot())
	return session, m.db.SaveSession(ctx, session)
}

func credentials(ctx context.Context, provider store.StoreProvider, result LoginResult) (store.StoreAuth, error) {
	if exchanger, ok := provider.(store.CodeExchanger); ok {
		if code := extractAuthorizationCode(result); code != "" {
			auth, err := exchanger.ExchangeCode(ctx, code)
			if err != nil {
				return store.StoreAuth{}, fmt.Errorf("troca do código de %s falhou: %w", provider.Name(), err)
			}
			auth.Cookies = store.FormatCookies(result.Cookies)
			return auth, nil
		}
		if token := firstCookie(result.Cookies, "access_token", "EPIC_BEARER_TOKEN"); token != "" {
			return store.StoreAuth{
				AccessToken: token,
				Cookies:     store.FormatCookies(result.Cookies),
			}, nil
		}
		return store.StoreAuth{}, fmt.Errorf("login de %s: código de autorização não encontrado", provider.Name())
	}

	if cookieAuth, ok := provider.(store.CookieAuthenticator); ok {
		return cookieAuth.AuthFromCookies(result.Cookies)
	}
	return store.StoreAuth{}, fmt.Errorf("login de %s: loja não suporta autenticação por WebView", provider.Name())
}

// extractAuthorizationCode procura o code no redirect (?code=...), no corpo
// JSON da Epic ({"authorizationCode": "..."}) ou nos cookies da sessão.
func extractAuthorizationCode(result LoginResult) string {
	if result.RedirectURL != "" {
		for _, key := range []string{"code", "authorizationCode", "exchange_code"} {
			if value := queryValue(result.RedirectURL, key); value != "" {
				return value
			}
		}
	}

	body := strings.TrimSpace(result.Body)
	if body != "" {
		var payload map[string]any
		if json.Unmarshal([]byte(body), &payload) == nil {
			for _, key := range []string{"authorizationCode", "code", "exchange_code"} {
				if value, ok := payload[key].(string); ok && value != "" {
					return value
				}
			}
		}
		for _, key := range []string{"authorizationCode", "exchange_code"} {
			if value := jsonStringField(body, key); value != "" {
				return value
			}
		}
	}

	for _, name := range []string{"exchange_code", "code"} {
		if value := result.Cookies[name]; value != "" {
			return value
		}
	}
	return ""
}

func firstCookie(cookies map[string]string, names ...string) string {
	for _, name := range names {
		if value := cookies[name]; value != "" {
			return value
		}
	}
	return ""
}

func queryValue(rawURL, key string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get(key)
}

func jsonStringField(body, key string) string {
	idx := strings.Index(body, `"`+key+`"`)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(body[idx+len(key)+2:])
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colon+1:])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
