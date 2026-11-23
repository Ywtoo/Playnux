package store

import (
	"context"
	"time"

	"github.com/playnux/core/models"
)

// StoreAuth representa as credenciais e tokens para consultar a conta digital da loja.
type StoreAuth struct {
	AccountID    string    `json:"account_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Cookies      string    `json:"cookies"`
	APIKey       string    `json:"api_key"`
}

// StoreProvider é a interface que cada loja implementa para sincronizar a biblioteca
// digital completa (jogos comprados que estão apenas na nuvem + instalados).
type StoreProvider interface {
	// ID identifica o provedor (ex: "steam", "gog", "epic")
	ID() string

	// Name é o nome legível da loja (ex: "Steam", "GOG Galaxy", "Epic Games Store")
	Name() string

	// Authenticate realiza o fluxo de login ou validação do token
	Authenticate(ctx context.Context, auth StoreAuth) error

	// IsAuthenticated verifica se a sessão digital está ativa
	IsAuthenticated() bool

	// FetchLibrary busca TODOS os jogos pertencentes à conta digital (instalados ou não)
	FetchLibrary(ctx context.Context) ([]*models.Game, error)

	// LaunchCommand retorna o comando ou URI do SO para iniciar o jogo
	LaunchCommand(game *models.Game) (string, []string, error)

	// UninstallCommand retorna a ação para desinstalar o jogo
	UninstallCommand(game *models.Game) (string, []string, error)
}

// WebLoginSpec descreve o fluxo de login web de uma loja. A ponte Wails usa esses
// dados para abrir a janela, saber quando o redirect de sucesso terminou e quais
// cookies precisam ser capturados.
type WebLoginSpec struct {
	URL               string
	CookieNames       []string
	RedirectURLPrefix string
	UnavailableReason string
}

// WebAuthenticator é implementada por lojas cujo login acontece em uma WebView.
type WebAuthenticator interface {
	WebLogin() WebLoginSpec
}

// CodeExchanger é implementada por lojas que devolvem um código de autorização
// no redirect (Epic e GOG). O Core troca o código pelos tokens de acesso.
type CodeExchanger interface {
	ExchangeCode(ctx context.Context, code string) (StoreAuth, error)
}

// CookieAuthenticator é implementada por lojas cuja sessão digital é comprovada
// por cookies (Steam).
type CookieAuthenticator interface {
	AuthFromCookies(cookies map[string]string) (StoreAuth, error)
}

// AuthSnapshotter expõe o estado atual das credenciais, incluindo tokens
// renovados durante a sincronização, para que o Core os persista.
type AuthSnapshotter interface {
	AuthSnapshot() StoreAuth
}

// LocalProvider marca fontes que não exigem conta nem sessão (jogos .desktop,
// Flatpak, pastas DRM-free e ROMs). Elas aparecem conectadas e sincronizam
// apenas lendo o disco.
type LocalProvider interface {
	IsLocal() bool
}

// InstalledImporter é implementada por lojas que conseguem importar somente os
// jogos instalados localmente (Steam/ACF, Heroic/Legendary) sem login web.
type InstalledImporter interface {
	FetchInstalled(ctx context.Context) ([]*models.Game, error)
}

// AccountIDAuthenticator permite conectar uma conta apenas pelo identificador
// público (SteamID64), sem senha ou token.
type AccountIDAuthenticator interface {
	AuthenticateAccountID(ctx context.Context, accountID string) error
}

// ApplicationID names an external client without exposing machine-specific paths.
type ApplicationID string

const (
	AppSteam     ApplicationID = "steam"
	AppHeroic    ApplicationID = "heroic"
	AppLegendary ApplicationID = "legendary"
	AppWine      ApplicationID = "wine"
)

// ApplicationCommand is the Core capability supplied to built-in providers.
type ApplicationCommand func(id ApplicationID, arguments ...string) (command string, args []string, found bool)

type ApplicationProvider interface {
	ApplicationRequirements() []ApplicationID
	SetApplicationCommand(ApplicationCommand)
}
