package epic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
	"github.com/playnux/core/store/builtins/heroic"
)

// Credenciais públicas do launcherAppClient2, usadas por todos os clientes
// open source da Epic (Legendary, Heroic) para o fluxo de authorization code.
const (
	epicClientID     = "34a02cf8f4414e29b15921876da36f9a"
	epicClientSecret = "daafbccc737745039dffe53d94fc76cf"

	epicOAuthBase   = "https://account-public-service-prod03.ol.epicgames.com"
	epicLibraryBase = "https://library-service.live.use1a.on.epicgames.com"

	epicCoverTypes      = "DieselGameBoxTall"
	epicBackgroundTypes = "DieselGameBox"
)

type EpicProvider struct {
	applicationCommand store.ApplicationCommand
	auth               store.StoreAuth
	client             *http.Client
	oauthBase          string
	libraryBase        string
}

func NewEpicProvider() *EpicProvider {
	return &EpicProvider{
		client:      &http.Client{Timeout: 20 * time.Second},
		oauthBase:   epicOAuthBase,
		libraryBase: epicLibraryBase,
	}
}

func (p *EpicProvider) ApplicationRequirements() []store.ApplicationID {
	return []store.ApplicationID{store.AppHeroic, store.AppLegendary}
}

func (p *EpicProvider) SetApplicationCommand(resolve store.ApplicationCommand) {
	p.applicationCommand = resolve
}

func (e *EpicProvider) ID() string { return "epic" }

func (e *EpicProvider) Name() string { return "Epic Games Store" }

// WebLogin descreve a janela oficial de login da Epic para a ponte WebView.
func (e *EpicProvider) WebLogin() store.WebLoginSpec {
	redirect := "https://www.epicgames.com/id/api/redirect?clientId=" +
		epicClientID + "&responseType=code"
	return store.WebLoginSpec{
		URL:               "https://www.epicgames.com/id/login?redirectUrl=" + url.QueryEscape(redirect),
		CookieNames:       []string{"EPIC_BEARER_TOKEN", "exchange_code", "access_token"},
		RedirectURLPrefix: "https://www.epicgames.com/id/api/redirect",
	}
}

func (e *EpicProvider) Authenticate(ctx context.Context, auth store.StoreAuth) error {
	if auth.AccessToken == "" && auth.RefreshToken == "" {
		return fmt.Errorf("epic: access token ou refresh token obrigatório")
	}

	e.auth = auth
	if e.auth.AccessToken == "" || tokenExpired(e.auth.ExpiresAt) {
		if e.auth.RefreshToken == "" {
			return fmt.Errorf("epic: token expirado e sem refresh token")
		}
		return e.refresh(ctx)
	}
	return nil
}

func (e *EpicProvider) IsAuthenticated() bool {
	return e.auth.AccessToken != "" || e.auth.RefreshToken != ""
}

// AuthSnapshot devolve os tokens atuais (podem ter sido renovados).
func (e *EpicProvider) AuthSnapshot() store.StoreAuth { return e.auth }

// ExchangeCode troca o authorization code capturado na WebView pelos tokens.
func (e *EpicProvider) ExchangeCode(ctx context.Context, code string) (store.StoreAuth, error) {
	return e.requestToken(ctx, url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
		"token_type": {"eg1"},
	})
}

func (e *EpicProvider) refresh(ctx context.Context) error {
	auth, err := e.requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {e.auth.RefreshToken},
		"token_type":    {"eg1"},
	})
	if err != nil {
		return err
	}
	// A Epic pode não devolver um novo refresh token; preserva o atual.
	if auth.RefreshToken == "" {
		auth.RefreshToken = e.auth.RefreshToken
	}
	if auth.AccountID == "" {
		auth.AccountID = e.auth.AccountID
	}
	auth.Cookies = e.auth.Cookies
	e.auth = auth
	return nil
}

type epicTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    string `json:"expires_at"`
	AccountID    string `json:"account_id"`
}

func (e *EpicProvider) requestToken(ctx context.Context, form url.Values) (store.StoreAuth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.oauthBase+"/account/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return store.StoreAuth{}, err
	}
	req.SetBasicAuth(epicClientID, epicClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.client.Do(req)
	if err != nil {
		return store.StoreAuth{}, fmt.Errorf("epic oauth: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return store.StoreAuth{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return store.StoreAuth{}, fmt.Errorf("epic oauth retornou status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token epicTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return store.StoreAuth{}, err
	}
	if token.AccessToken == "" {
		return store.StoreAuth{}, fmt.Errorf("epic oauth: resposta sem access_token")
	}

	expiresAt := tokenExpiry(token)
	return store.StoreAuth{
		AccountID:    token.AccountID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func tokenExpiry(token epicTokenResponse) time.Time {
	if token.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	if token.ExpiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, token.ExpiresAt); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func tokenExpired(expiresAt time.Time) bool {
	return !expiresAt.IsZero() && time.Now().After(expiresAt.Add(-time.Minute))
}

// FetchLibrary mescla a biblioteca local (Heroic/Legendary) com a biblioteca
// digital completa da conta Epic.
func (e *EpicProvider) FetchLibrary(ctx context.Context) ([]*models.Game, error) {
	games := make(map[string]*models.Game)
	for _, entry := range heroic.EpicLibrary() {
		game := gameFromLocal(entry)
		games[game.GameID] = game
	}

	if e.IsAuthenticated() {
		if err := e.ensureToken(ctx); err != nil {
			return nil, err
		}
		digital, err := e.fetchDigitalLibrary(ctx)
		if err != nil {
			return nil, err
		}
		for _, game := range digital {
			if local, ok := games[game.GameID]; ok {
				mergeInstallState(game, local)
			}
			games[game.GameID] = game
		}
	}

	return sortedGames(games), nil
}

// FetchInstalled importa somente os jogos que o Heroic/Legendary conhece no
// disco, sem exigir login na Epic.
func (e *EpicProvider) FetchInstalled(context.Context) ([]*models.Game, error) {
	entries := heroic.EpicLibrary()
	games := make([]*models.Game, 0, len(entries))
	for _, entry := range entries {
		games = append(games, gameFromLocal(entry))
	}
	return games, nil
}

func (e *EpicProvider) ensureToken(ctx context.Context) error {
	if e.auth.AccessToken != "" && !tokenExpired(e.auth.ExpiresAt) {
		return nil
	}
	if e.auth.RefreshToken == "" {
		return fmt.Errorf("epic: sessão expirada, faça login novamente")
	}
	return e.refresh(ctx)
}

type epicRecordList struct {
	Records          []epicRecord `json:"records"`
	ResponseMetadata struct {
		NextCursor string `json:"nextCursor"`
	} `json:"responseMetadata"`
}

type epicRecord struct {
	ProductID     string `json:"productId"`
	AppName       string `json:"appName"`
	CatalogItemID string `json:"catalogItemId"`
	Namespace     string `json:"namespace"`
	ProductName   string `json:"productName"`
	Metadata      *struct {
		Title           string         `json:"title"`
		Description     string         `json:"description"`
		LongDescription string         `json:"longDescription"`
		Developer       string         `json:"developer"`
		KeyImages       []heroic.Image `json:"keyImages"`
	} `json:"metadata"`
}

func (e *EpicProvider) fetchDigitalLibrary(ctx context.Context) ([]*models.Game, error) {
	var games []*models.Game
	cursor := ""
	seen := make(map[string]bool)

	for {
		endpoint := e.libraryBase + "/library/api/public/items?includeMetadata=true"
		if cursor != "" {
			endpoint += "&cursor=" + url.QueryEscape(cursor)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "bearer "+e.auth.AccessToken)

		resp, err := e.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("epic library: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("epic library retornou status %d: %s",
				resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page epicRecordList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		for _, record := range page.Records {
			if game := gameFromRecord(record); game != nil {
				games = append(games, game)
			}
		}

		next := page.ResponseMetadata.NextCursor
		if next == "" || seen[next] {
			break
		}
		seen[next] = true
		cursor = next
	}

	return games, nil
}

func gameFromRecord(record epicRecord) *models.Game {
	gameID := record.AppName
	if gameID == "" {
		gameID = record.CatalogItemID
	}
	if gameID == "" {
		return nil
	}

	name := record.ProductName
	var description, developer string
	var images []heroic.Image
	if record.Metadata != nil {
		if record.Metadata.Title != "" {
			name = record.Metadata.Title
		}
		description = record.Metadata.Description
		if description == "" {
			description = record.Metadata.LongDescription
		}
		developer = record.Metadata.Developer
		images = record.Metadata.KeyImages
	}
	if name == "" {
		name = gameID
	}

	game := &models.Game{
		GameID:          gameID,
		PluginID:        "epic",
		Source:          "Epic",
		Name:            name,
		SortingName:     name,
		CoverImage:      heroic.ImageURL(images, epicCoverTypes, "OfferImageTall"),
		BackgroundImage: heroic.ImageURL(images, epicBackgroundTypes, "DieselStoreFrontWide", "OfferImageWide"),
		Description:     description,
		Platforms:       []string{"PC (Windows)"},
	}
	if developer != "" {
		game.Developers = []string{developer}
	}
	return game
}

func gameFromLocal(entry *heroic.Entry) *models.Game {
	platforms := []string{"PC (Windows)"}
	if entry.Install != nil && strings.EqualFold(entry.Install.Platform, "linux") {
		platforms = []string{"PC (Linux)"}
	}

	game := &models.Game{
		GameID:          entry.AppName,
		PluginID:        "epic",
		Source:          "Epic",
		Name:            entry.Title,
		SortingName:     entry.Title,
		CoverImage:      entry.Cover(),
		BackgroundImage: entry.Background(),
		Platforms:       platforms,
	}

	if entry.Installed() {
		game.IsInstalled = true
		game.InstallDirectory = entry.Install.Path
		game.InstallSize = entry.Install.Size
	}
	return game
}

func mergeInstallState(dst, local *models.Game) {
	dst.IsInstalled = local.IsInstalled
	dst.InstallDirectory = local.InstallDirectory
	dst.InstallSize = local.InstallSize
	if dst.CoverImage == "" {
		dst.CoverImage = local.CoverImage
	}
	if dst.BackgroundImage == "" {
		dst.BackgroundImage = local.BackgroundImage
	}
	for _, platform := range local.Platforms {
		ensurePlatform(dst, platform)
	}
}

func ensurePlatform(game *models.Game, platform string) {
	for _, existing := range game.Platforms {
		if existing == platform {
			return
		}
	}
	game.Platforms = append(game.Platforms, platform)
}

func sortedGames(games map[string]*models.Game) []*models.Game {
	result := make([]*models.Game, 0, len(games))
	for _, game := range games {
		result = append(result, game)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (e *EpicProvider) LaunchCommand(game *models.Game) (string, []string, error) {
	if game == nil || game.GameID == "" {
		return "", nil, fmt.Errorf("epic: jogo sem app_name")
	}

	if e.applicationCommand != nil {
		if command, args, found := e.applicationCommand(store.AppHeroic, heroic.LaunchURI("legendary", game.GameID)); found {
			return command, args, nil
		}
		if command, args, found := e.applicationCommand(store.AppLegendary, "launch", game.GameID); found {
			return command, args, nil
		}
	}
	return "", nil, fmt.Errorf("epic: instale o Heroic Games Launcher ou o Legendary para lançar jogos")
}

func (e *EpicProvider) UninstallCommand(game *models.Game) (string, []string, error) {
	if game == nil || game.GameID == "" {
		return "", nil, fmt.Errorf("epic: jogo sem app_name")
	}
	if e.applicationCommand != nil {
		if command, args, found := e.applicationCommand(store.AppLegendary, "uninstall", game.GameID, "--yes"); found {
			return command, args, nil
		}
	}
	return "", nil, fmt.Errorf("epic: desinstalação requer o Legendary CLI (o Heroic não expõe URI de desinstalação)")
}
