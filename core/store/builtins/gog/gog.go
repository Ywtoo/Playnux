package gog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
	"github.com/playnux/core/store/builtins/heroic"
)

// Credenciais públicas do cliente Galaxy (usadas por GOGdl, Heroic, Minigalaxy
// e lgogdownloader há mais de uma década).
const (
	gogClientID     = "46899977096215655"
	gogClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
	gogRedirectURI  = "https://embed.gog.com/on_login_success?origin=client"

	gogAuthBase  = "https://auth.gog.com"
	gogEmbedBase = "https://embed.gog.com"
)

type GOGProvider struct {
	applicationCommand store.ApplicationCommand
	auth               store.StoreAuth
	client             *http.Client
	authBase           string
	embedBase          string
	productBase        string
}

func NewGOGProvider() *GOGProvider {
	return &GOGProvider{
		client:      &http.Client{Timeout: 20 * time.Second},
		authBase:    gogAuthBase,
		embedBase:   gogEmbedBase,
		productBase: "https://api.gog.com",
	}
}

func (p *GOGProvider) ApplicationRequirements() []store.ApplicationID {
	return []store.ApplicationID{store.AppHeroic}
}

func (p *GOGProvider) SetApplicationCommand(resolve store.ApplicationCommand) {
	p.applicationCommand = resolve
}

func (g *GOGProvider) ID() string { return "gog" }

func (g *GOGProvider) Name() string { return "GOG Galaxy" }

func (g *GOGProvider) WebLogin() store.WebLoginSpec {
	return store.WebLoginSpec{
		URL: g.authBase + "/auth?client_id=" + gogClientID +
			"&redirect_uri=" + url.QueryEscape(gogRedirectURI) +
			"&response_type=code&layout=galaxy",
		RedirectURLPrefix: "https://embed.gog.com/on_login_success",
	}
}

func (g *GOGProvider) Authenticate(ctx context.Context, auth store.StoreAuth) error {
	if auth.AccessToken == "" && auth.RefreshToken == "" {
		return fmt.Errorf("gog: access token ou refresh token obrigatório")
	}

	auth.Cookies = ""
	g.auth = auth
	if g.auth.AccessToken == "" || tokenExpired(g.auth.ExpiresAt) {
		if g.auth.RefreshToken == "" {
			return fmt.Errorf("gog: token expirado e sem refresh token")
		}
		return g.refresh(ctx)
	}
	return nil
}

func (g *GOGProvider) IsAuthenticated() bool {
	return g.auth.AccessToken != "" || g.auth.RefreshToken != ""
}

// AuthSnapshot devolve os tokens atuais (podem ter sido renovados).
func (g *GOGProvider) AuthSnapshot() store.StoreAuth { return g.auth }

// ExchangeCode troca o code do redirect de login pelos tokens GOG.
func (g *GOGProvider) ExchangeCode(ctx context.Context, code string) (store.StoreAuth, error) {
	return g.requestToken(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {gogRedirectURI},
	})
}

func (g *GOGProvider) refresh(ctx context.Context) error {
	auth, err := g.requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {g.auth.RefreshToken},
	})
	if err != nil {
		return err
	}
	if auth.RefreshToken == "" {
		auth.RefreshToken = g.auth.RefreshToken
	}
	if auth.AccountID == "" {
		auth.AccountID = g.auth.AccountID
	}
	g.auth = auth
	return nil
}

type gogTokenResponse struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	ExpiresIn    int64      `json:"expires_in"`
	UserID       flexString `json:"user_id"`
}

func (g *GOGProvider) requestToken(ctx context.Context, form url.Values) (store.StoreAuth, error) {
	query := url.Values{
		"client_id":     {gogClientID},
		"client_secret": {gogClientSecret},
	}
	for key, values := range form {
		query[key] = values
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.authBase+"/token?"+query.Encode(), nil)
	if err != nil {
		return store.StoreAuth{}, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return store.StoreAuth{}, fmt.Errorf("gog oauth: falha na comunicação com o servidor")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return store.StoreAuth{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return store.StoreAuth{}, fmt.Errorf("gog oauth retornou status %d", resp.StatusCode)
	}

	var token gogTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return store.StoreAuth{}, err
	}
	if token.AccessToken == "" {
		return store.StoreAuth{}, fmt.Errorf("gog oauth: resposta sem access_token")
	}

	var expiresAt time.Time
	if token.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return store.StoreAuth{
		AccountID:    string(token.UserID),
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func tokenExpired(expiresAt time.Time) bool {
	return !expiresAt.IsZero() && time.Now().After(expiresAt.Add(-time.Minute))
}

// FetchLibrary mescla os jogos instalados/em cache no Heroic com todos os
// produtos da conta digital (GET /user/data/games).
func (g *GOGProvider) FetchLibrary(ctx context.Context) ([]*models.Game, error) {
	games := make(map[string]*models.Game)
	for _, entry := range heroic.GOGLibrary() {
		game := gameFromLocal(entry)
		if !IsLibraryExtra(game) {
			games[game.GameID] = game
		}
	}

	if !g.IsAuthenticated() {
		return sortedGames(games), nil
	}
	if err := g.ensureToken(ctx); err != nil {
		return nil, err
	}

	owned, err := g.ownedGameIDs(ctx)
	if err != nil {
		return nil, err
	}

	details, detailErr := g.fetchDetails(ctx, owned)
	if detailErr != nil && len(details) == 0 {
		return nil, detailErr
	}
	for id, detail := range details {
		game := gameFromRecord(id, detail)
		if game == nil {
			continue
		}
		if local, ok := games[id]; ok {
			mergeInstallState(game, local)
		}
		games[id] = game
	}

	return sortedGames(games), nil
}

// FetchInstalled importa somente a biblioteca local conhecida pelo Heroic,
// sem exigir token da GOG.
func (g *GOGProvider) FetchInstalled(context.Context) ([]*models.Game, error) {
	entries := heroic.GOGLibrary()
	games := make([]*models.Game, 0, len(entries))
	for _, entry := range entries {
		if game := gameFromLocal(entry); game.IsInstalled && !IsLibraryExtra(game) {
			games = append(games, game)
		}
	}
	return games, nil
}

func (g *GOGProvider) ensureToken(ctx context.Context) error {
	if g.auth.AccessToken != "" && !tokenExpired(g.auth.ExpiresAt) {
		return nil
	}
	if g.auth.RefreshToken == "" {
		return fmt.Errorf("gog: sessão expirada, faça login novamente")
	}
	return g.refresh(ctx)
}

func (g *GOGProvider) ownedGameIDs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.embedBase+"/user/data/games", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.auth.AccessToken)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gog library: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gog library retornou status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var owned struct {
		Owned []int64 `json:"owned"`
	}
	if err := json.Unmarshal(body, &owned); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(owned.Owned))
	for _, id := range owned.Owned {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	return ids, nil
}

type gogDetails struct {
	Title            string `json:"title"`
	ProductType      string `json:"productType"`
	IsDLC            bool   `json:"isDlc"`
	Image            string `json:"image"`
	BackgroundImage  string `json:"backgroundImage"`
	Description      string `json:"description"`
	ReleaseTimestamp int64  `json:"releaseTimestamp"`
	Genres           []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Developers []struct {
		Name string `json:"name"`
	} `json:"developers"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
}

// fetchDetails busca os detalhes em paralelo (máximo de 8 requisições) para
// não transformar uma biblioteca grande em centenas de chamadas sequenciais.
func (g *GOGProvider) fetchDetails(ctx context.Context, ids []string) (map[string]*gogDetails, error) {
	results := make(map[string]*gogDetails)
	if len(ids) == 0 {
		return results, nil
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			details, err := g.gameDetails(ctx, id)
			if err != nil || details.ProductType == "" {
				if product, catalogErr := g.productDetails(ctx, id); catalogErr == nil {
					if err != nil {
						details, err = product, nil
					} else {
						details.ProductType = product.ProductType
						if goodTitle(product.Title, id) {
							details.Title = product.Title
						}
						if details.Image == "" {
							details.Image = product.Image
						}
						if details.BackgroundImage == "" {
							details.BackgroundImage = product.BackgroundImage
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			results[id] = details
		}(id)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, firstErr
}

func (g *GOGProvider) gameDetails(ctx context.Context, id string) (*gogDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.embedBase+"/account/gameDetails/"+url.PathEscape(id)+".json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.auth.AccessToken)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gog gameDetails %s retornou status %d", id, resp.StatusCode)
	}

	var details gogDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}
	return &details, nil
}

func (g *GOGProvider) productDetails(ctx context.Context, id string) (*gogDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.productBase+"/products/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	response, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GOG catalogue: status %d", response.StatusCode)
	}
	var product struct {
		Title       string `json:"title"`
		GameType    string `json:"game_type"`
		ProductType string `json:"productType"`
		Images      struct {
			BoxArt     string `json:"boxArt"`
			Background string `json:"background"`
		} `json:"images"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&product); err != nil {
		return nil, err
	}
	kind := product.GameType
	if kind == "" {
		kind = product.ProductType
	}
	return &gogDetails{Title: product.Title, ProductType: kind, Image: product.Images.BoxArt,
		BackgroundImage: product.Images.Background}, nil
}

func goodTitle(name, id string) bool {
	title := strings.TrimSpace(name)
	return title != "" && title != id
}

func isExtraTitle(name string) bool {
	title := strings.ToLower(strings.TrimSpace(name))
	for _, suffix := range []string{" soundtrack", " original soundtrack", " (soundtrack)", " - soundtrack", " artbook", " art book", " digital extras", " bonus content", " goodies"} {
		if strings.HasSuffix(title, suffix) {
			return true
		}
	}
	return false
}

func IsLibraryExtra(game *models.Game) bool {
	return game.PluginID == "gog" && !game.IsCustomGame && !game.IsInstalled &&
		(!goodTitle(game.Name, game.GameID) || isExtraTitle(game.Name))
}

func gameFromRecord(id string, details *gogDetails) *models.Game {
	name := strings.TrimSpace(details.Title)
	if !goodTitle(name, id) || isExtraTitle(name) || details.IsDLC {
		return nil
	}
	switch strings.ToLower(details.ProductType) {
	case "dlc", "bonus", "soundtrack", "movie", "video", "extra", "pack":
		return nil
	}

	game := &models.Game{
		GameID:          id,
		PluginID:        "gog",
		Source:          "GOG",
		Name:            name,
		SortingName:     name,
		CoverImage:      heroic.NormalizeURL(details.Image),
		BackgroundImage: heroic.NormalizeURL(details.BackgroundImage),
		Description:     details.Description,
		Platforms:       []string{"PC (Windows)"},
	}

	if details.ReleaseTimestamp > 0 {
		release := time.Unix(details.ReleaseTimestamp, 0)
		game.ReleaseDate = &release
		game.ReleaseYear = release.Year()
	}
	for _, genre := range details.Genres {
		if genre.Name != "" {
			game.Genres = append(game.Genres, genre.Name)
		}
	}
	for _, developer := range details.Developers {
		if developer.Name != "" {
			game.Developers = append(game.Developers, developer.Name)
		}
	}
	for _, publisher := range details.Publishers {
		if publisher.Name != "" {
			game.Publishers = append(game.Publishers, publisher.Name)
		}
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
		PluginID:        "gog",
		Source:          "GOG",
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

func (g *GOGProvider) LaunchCommand(game *models.Game) (string, []string, error) {
	if game == nil || game.GameID == "" {
		return "", nil, fmt.Errorf("gog: jogo sem product id")
	}

	if g.applicationCommand != nil {
		if command, args, found := g.applicationCommand(store.AppHeroic, heroic.LaunchURI("gog", game.GameID)); found {
			return command, args, nil
		}
	}

	// Fallback: build nativo Linux instalado sem Heroic.
	if install := heroic.InstallFor(game.GameID); install != nil && strings.EqualFold(install.Platform, "linux") {
		if executable := install.ExecutablePath(); executable != "" {
			return executable, nil, nil
		}
	}
	return "", nil, fmt.Errorf("gog: instale o Heroic Games Launcher para lançar jogos GOG no Linux")
}

func (g *GOGProvider) UninstallCommand(game *models.Game) (string, []string, error) {
	if game == nil || game.GameID == "" {
		return "", nil, fmt.Errorf("gog: jogo sem product id")
	}

	install := heroic.InstallFor(game.GameID)
	dir := ""
	if install != nil {
		dir = install.Path
	}
	if dir == "" && game.IsInstalled {
		dir = game.InstallDirectory
	}
	if dir == "" {
		return "", nil, fmt.Errorf("gog: jogo não está instalado")
	}

	uninstaller := findUninstaller(dir)
	if uninstaller == "" {
		return "", nil, fmt.Errorf("gog: nenhum desinstalador encontrado em %s (remova pelo Heroic)", dir)
	}

	wineBinary := ""
	if install != nil {
		wineBinary = install.WineBinary
	}
	if wineBinary == "" {
		found := false
		if g.applicationCommand != nil {
			wineBinary, _, found = g.applicationCommand(store.AppWine)
		}
		if !found {
			return "", nil, fmt.Errorf("gog: Wine não encontrado para executar o desinstalador")
		}
	}

	if install != nil && install.WinePrefix != "" {
		return "env", []string{
			"WINEPREFIX=" + install.WinePrefix,
			wineBinary,
			uninstaller,
		}, nil
	}
	return wineBinary, []string{uninstaller}, nil
}

func findUninstaller(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(entry.Name()) {
		case "unins000.exe", "unins001.exe", "uninstall.exe", "uninstaller.exe":
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

// flexString aceita string ou número (a GOG devolve user_id como string).
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	value := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if value == "null" {
		value = ""
	}
	*f = flexString(value)
	return nil
}
