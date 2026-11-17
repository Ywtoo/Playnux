package steam

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

type SteamProvider struct {
	applicationCommand store.ApplicationCommand
	auth               store.StoreAuth
	communityBase      string
}

func NewSteamProvider() *SteamProvider {
	return &SteamProvider{communityBase: "https://steamcommunity.com"}
}

func (p *SteamProvider) ApplicationRequirements() []store.ApplicationID {
	return []store.ApplicationID{store.AppSteam}
}

func (p *SteamProvider) SetApplicationCommand(resolve store.ApplicationCommand) {
	p.applicationCommand = resolve
}

func (s *SteamProvider) ID() string {
	return "steam"
}

func (s *SteamProvider) Name() string {
	return "Steam"
}

func (s *SteamProvider) Authenticate(ctx context.Context, auth store.StoreAuth) error {
	s.auth = auth
	if s.auth.AccountID == "" {
		detectedID, err := detectLocalSteamID64()
		if err == nil && detectedID != "" {
			s.auth.AccountID = detectedID
		}
	}
	return nil
}

func (s *SteamProvider) IsAuthenticated() bool {
	return s.auth.AccountID != "" || s.auth.APIKey != ""
}

// WebLogin descreve a janela oficial de login da Steam para a ponte WebView.
func (s *SteamProvider) WebLogin() store.WebLoginSpec {
	return store.WebLoginSpec{
		URL:               "https://steamcommunity.com/login/home/?goto=",
		CookieNames:       []string{"steamLoginSecure"},
		RedirectURLPrefix: "https://steamcommunity.com/",
	}
}

// AuthFromCookies extrai o SteamID64 do cookie steamLoginSecure
// (formato "<steamid>%7C%7C<token>").
func (s *SteamProvider) AuthFromCookies(cookies map[string]string) (store.StoreAuth, error) {
	raw := cookies["steamLoginSecure"]
	if raw == "" {
		return store.StoreAuth{}, fmt.Errorf("steam: cookie steamLoginSecure não capturado")
	}

	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		decoded = raw
	}
	accountID, _, _ := strings.Cut(decoded, "||")
	if accountID == "" {
		return store.StoreAuth{}, fmt.Errorf("steam: steamLoginSecure sem SteamID64")
	}

	return store.StoreAuth{
		AccountID: accountID,
		Cookies:   store.FormatCookies(cookies),
	}, nil
}

// SteamOwnedGamesResponse mapeia a resposta do endpoint IPlayerService/GetOwnedGames
type SteamOwnedGamesResponse struct {
	Response struct {
		GameCount int `json:"game_count"`
		Games     []struct {
			AppID                  int    `json:"appid"`
			Name                   string `json:"name"`
			PlaytimeForever        int    `json:"playtime_forever"` // minutos
			ImgIconURL             string `json:"img_icon_url"`
			RTimeLastPlayed        int64  `json:"rtime_last_played"`
			PlaytimeWindowsForever int    `json:"playtime_windows_forever"`
			PlaytimeMacForever     int    `json:"playtime_mac_forever"`
			PlaytimeLinuxForever   int    `json:"playtime_linux_forever"`
		} `json:"games"`
	} `json:"response"`
}

// FetchLibrary sincroniza tanto a biblioteca digital completa (via Web API / Perfil)
// quanto os jogos atualmente baixados no disco Linux via arquivos .acf.
// nonGameAppIDs lists Steam runtimes, compatibility tools, SDKs and other
// redistributables that ship with .acf manifests but are not playable games.
var nonGameAppIDs = map[string]bool{
	"228980":  true, // Steamworks Common Redistributables
	"891390":  true, // Proton 7.0
	"1054230": true, // Proton 8.0
	"1113280": true, // Proton 9.0 (beta)
	"1245040": true, // Proton 9.0
	"1391110": true, // Steam Linux Runtime - Soldier
	"1420170": true, // Proton 8.0 (beta)
	"1493710": true, // Proton Experimental
	"1580130": true, // Proton 8.0 (ARM64)
	"1628350": true, // Steam Linux Runtime - Sniper
	"1887720": true, // Proton 7.0 (beta)
	"2348590": true, // Proton 9.0 (beta)
	"1070560": true, // Steam Linux Runtime - Scout
	"961940":  true, // Steam Linux Runtime (legacy)
}

// nonGameWords filters titles that are clearly tools, servers or runtimes even
// when a manifest exists. The match is case-insensitive.
var nonGameWords = []string{
	"proton", "steam linux runtime", "steamworks common", "dedicated server",
	"sdk", "tool", "benchmark", "editor", "authoring tools",
	"redistributable", "soundtrack", "demo",
}

// IsPlayableGame reports whether a Steam app should be imported automatically.
// Tools and runtimes can still be added manually as local applications.
func IsPlayableGame(appID, name string) bool {
	if nonGameAppIDs[strings.TrimSpace(appID)] {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	for _, word := range nonGameWords {
		if strings.Contains(lower, word) {
			return false
		}
	}
	return true
}

func (s *SteamProvider) FetchLibrary(ctx context.Context) ([]*models.Game, error) {
	installedMap := scanLocalInstalledGames()
	gameMap := make(map[string]*models.Game)

	// 1. Busca a biblioteca comprada. Com API key usa o Web API oficial; sem
	// key tenta o XML público do perfil (quando os detalhes de jogo permitem).
	if s.auth.AccountID != "" {
		var digitalGames []*models.Game
		var err error
		if s.auth.APIKey != "" {
			digitalGames, err = s.fetchDigitalLibrary(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			digitalGames, err = s.fetchCommunityLibrary(ctx)
			// Perfil privado não pode impedir a importação dos instalados.
			if err != nil {
				digitalGames = nil
			}
		}
		for _, game := range digitalGames {
			gameMap[game.GameID] = game
		}
	}

	// 2. Mescla com os jogos instalados localmente no Linux
	for appID, localGame := range installedMap {
		if existing, ok := gameMap[appID]; ok {
			existing.IsInstalled = true
			existing.InstallDirectory = localGame.InstallDirectory
			existing.InstallSize = localGame.InstallSize
			if localGame.Name != "" {
				existing.Name = localGame.Name
			}
		} else {
			gameMap[appID] = localGame
		}
	}

	var results []*models.Game
	for _, g := range gameMap {
		results = append(results, g)
	}

	return results, nil
}

// FetchInstalled lê apenas os manifests ACF locais, sem API ou login.
func (s *SteamProvider) FetchInstalled(context.Context) ([]*models.Game, error) {
	installed := scanLocalInstalledGames()
	games := make([]*models.Game, 0, len(installed))
	for _, game := range installed {
		games = append(games, game)
	}
	return games, nil
}

// AuthenticateAccountID permite conectar pelo SteamID64 público. Sem API key
// isso não busca a biblioteca na nuvem, mas libera o scanner local e o estado
// de conta conectada.
func (s *SteamProvider) AuthenticateAccountID(_ context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if err := ValidateAccountID(accountID); err != nil {
		return err
	}
	s.auth.AccountID = accountID
	return nil
}

func ValidateAccountID(accountID string) error {
	if len(accountID) != 17 || !strings.HasPrefix(accountID, "7656119") {
		return fmt.Errorf("steam: SteamID64 inválido (esperado 17 dígitos começando por 7656119)")
	}
	for _, char := range accountID {
		if char < '0' || char > '9' {
			return fmt.Errorf("steam: SteamID64 deve conter apenas números")
		}
	}
	return nil
}

func (s *SteamProvider) AuthSnapshot() store.StoreAuth { return s.auth }

type communityGamesXML struct {
	Games []struct {
		AppID         int    `xml:"appID"`
		Name          string `xml:"name"`
		Logo          string `xml:"logo"`
		HoursOnRecord string `xml:"hoursOnRecord"`
	} `xml:"games>game"`
}

// fetchCommunityLibrary lê a lista pública de jogos do perfil. Alguns perfis
// exigem login na Steam para expor essa página; nesse caso o chamador mantém a
// biblioteca instalada e orienta o usuário a informar uma API key.
func (s *SteamProvider) fetchCommunityLibrary(ctx context.Context) ([]*models.Game, error) {
	base := strings.TrimRight(s.communityBase, "/")
	if base == "" {
		base = "https://steamcommunity.com"
	}
	endpoint := base + "/profiles/" + url.PathEscape(s.auth.AccountID) + "/games?tab=all&xml=1"
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Playnux/1.0 (+https://github.com/playnux)")
	req.Header.Set("Accept", "application/xml,text/xml;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("consultar perfil Steam: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("perfil Steam retornou status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var parsed communityGamesXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("perfil Steam não expõe a lista de jogos: %w", err)
	}

	games := make([]*models.Game, 0, len(parsed.Games))
	for _, raw := range parsed.Games {
		appID := strconv.Itoa(raw.AppID)
		if raw.AppID <= 0 || !IsPlayableGame(appID, raw.Name) {
			continue
		}
		name := strings.TrimSpace(raw.Name)
		games = append(games, &models.Game{
			GameID:          appID,
			PluginID:        "steam",
			Source:          "Steam",
			Name:            name,
			SortingName:     name,
			CoverImage:      fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/library_600x900_2x.jpg", appID),
			BackgroundImage: fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/page_bg_generated_v6b.jpg", appID),
			Playtime:        hoursToSeconds(raw.HoursOnRecord),
			Platforms:       []string{"PC (Linux)", "PC (Windows)"},
		})
	}
	return games, nil
}

func hoursToSeconds(raw string) uint64 {
	hours, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || hours <= 0 {
		return 0
	}
	return uint64(hours * 3600)
}

func (s *SteamProvider) fetchDigitalLibrary(ctx context.Context) ([]*models.Game, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	query := url.Values{
		"key": {s.auth.APIKey}, "steamid": {s.auth.AccountID},
		"include_appinfo": {"1"}, "include_played_free_games": {"1"},
	}
	endpoint := "https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("não foi possível consultar a API Steam")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam api retornou status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp SteamOwnedGamesResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	var games []*models.Game
	for _, raw := range apiResp.Response.Games {
		appIDStr := fmt.Sprintf("%d", raw.AppID)
		if !IsPlayableGame(appIDStr, raw.Name) {
			continue
		}

		// URLs padronizadas de imagens oficiais da Valve
		cover := fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/library_600x900_2x.jpg", appIDStr)
		bg := fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/page_bg_generated_v6b.jpg", appIDStr)
		icon := ""
		if raw.ImgIconURL != "" {
			icon = fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%s/%s.jpg", appIDStr, raw.ImgIconURL)
		}

		var lastActivity *time.Time
		if raw.RTimeLastPlayed > 0 {
			t := time.Unix(raw.RTimeLastPlayed, 0)
			lastActivity = &t
		}

		g := &models.Game{
			GameID:          appIDStr,
			PluginID:        "steam",
			Source:          "Steam",
			Name:            raw.Name,
			SortingName:     raw.Name,
			Icon:            icon,
			CoverImage:      cover,
			BackgroundImage: bg,
			IsInstalled:     false,
			Playtime:        uint64(raw.PlaytimeForever * 60), // converte minutos para segundos
			LastActivity:    lastActivity,
			Platforms:       []string{"PC (Linux)", "PC (Windows)"},
		}
		games = append(games, g)
	}

	return games, nil
}

func (s *SteamProvider) LaunchCommand(game *models.Game) (string, []string, error) {
	return s.command(game, "rungameid")
}

func (s *SteamProvider) UninstallCommand(game *models.Game) (string, []string, error) {
	return s.command(game, "uninstall")
}

func (s *SteamProvider) command(game *models.Game, action string) (string, []string, error) {
	if game == nil || game.GameID == "" {
		return "", nil, fmt.Errorf("steam: jogo sem AppID")
	}
	if s.applicationCommand != nil {
		uri := fmt.Sprintf("steam://%s/%s", action, game.GameID)
		if command, args, found := s.applicationCommand(store.AppSteam, uri); found {
			return command, args, nil
		}
	}
	return "", nil, fmt.Errorf("instale a Steam para executar jogos da biblioteca Steam")
}

// ── Scanners Locais de Arquivos do Linux ─────────────────────────────────────

// DetectLocalAccountID expõe o SteamID64 do último usuário logado na Steam
// deste computador, sem abrir a janela de login.
func DetectLocalAccountID() (string, error) {
	return detectLocalSteamID64()
}

func detectLocalSteamID64() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	paths := []string{
		filepath.Join(home, ".steam/steam/config/loginusers.vdf"),
		filepath.Join(home, ".local/share/Steam/config/loginusers.vdf"),
		filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam/config/loginusers.vdf"),
	}

	for _, p := range paths {
		if content, err := os.ReadFile(p); err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				// Localiza chaves como "76561198000000000"
				if strings.HasPrefix(line, "\"7656119") {
					parts := strings.Split(line, "\"")
					if len(parts) >= 2 {
						return parts[1], nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("nenhum steamid64 local encontrado")
}

func scanLocalInstalledGames() map[string]*models.Game {
	results := make(map[string]*models.Game)
	home, err := os.UserHomeDir()
	if err != nil {
		return results
	}

	steamappsDirs := []string{
		filepath.Join(home, ".steam/steam/steamapps"),
		filepath.Join(home, ".local/share/Steam/steamapps"),
		filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam/steamapps"),
	}

	for _, sDir := range libraryDirectories(steamappsDirs) {
		entries, err := os.ReadDir(sDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "appmanifest_") && strings.HasSuffix(e.Name(), ".acf") {
				appID := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "appmanifest_"), ".acf")
				content, err := os.ReadFile(filepath.Join(sDir, e.Name()))
				if err != nil {
					continue
				}

				name, installDir, size := parseAppManifest(string(content))
				if name == "" || installDir == "" || !IsPlayableGame(appID, name) {
					continue
				}
				fullInstallPath := filepath.Join(sDir, "common", installDir)

				results[appID] = &models.Game{
					GameID:           appID,
					PluginID:         "steam",
					Source:           "Steam",
					Name:             name,
					SortingName:      name,
					CoverImage:       fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/library_600x900_2x.jpg", appID),
					BackgroundImage:  fmt.Sprintf("https://steamcdn-a.akamaihd.net/steam/apps/%s/page_bg_generated_v6b.jpg", appID),
					IsInstalled:      true,
					InstallDirectory: fullInstallPath,
					InstallSize:      size,
					Platforms:        []string{"PC (Linux)"},
				}
			}
		}
	}

	return results
}

// Steam has used both numbered path entries and nested "path" entries.
var libraryPathEntry = regexp.MustCompile(`(?m)^\s*"(?:path|[0-9]+)"[\t ]+"((?:\\.|[^"\\])*)"`)

func libraryDirectories(defaults []string) []string {
	dirs := make([]string, 0, len(defaults))
	seen := make(map[string]bool)
	add := func(dir string) {
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, dir := range defaults {
		add(dir)
		content, err := os.ReadFile(filepath.Join(dir, "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, match := range libraryPathEntry.FindAllSubmatch(content, -1) {
			path := strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(string(match[1]))
			if filepath.IsAbs(path) {
				add(filepath.Join(path, "steamapps"))
			}
		}
	}
	return dirs
}

func parseAppManifest(content string) (name string, installDir string, size uint64) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"name\"") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				name = parts[3]
			}
		} else if strings.HasPrefix(line, "\"installdir\"") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				installDir = parts[3]
			}
		} else if strings.HasPrefix(line, "\"SizeOnDisk\"") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				fmt.Sscanf(parts[3], "%d", &size)
			}
		}
	}
	return
}
