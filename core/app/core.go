// Package app é o serviço mestre do Core do Playnux. Ele orquestra banco,
// provedores, metadados e runners sem depender de Wails ou CGO.
//
// Toda a lógica de banco, provedores, login e lançamento fica aqui para que
// possa ser testada com `go test` normal; o pacote host só traduz chamadas
// Wails e cuida da WebView.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/playnux/core/auth"
	"github.com/playnux/core/database"
	"github.com/playnux/core/metadata"
	"github.com/playnux/core/metadata/steamgriddb"
	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/applications"
	"github.com/playnux/core/runner/gameproc"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/store"
	"github.com/playnux/core/store/builtins/emulation"
	"github.com/playnux/core/store/builtins/epic"
	"github.com/playnux/core/store/builtins/gog"
	"github.com/playnux/core/store/builtins/linux"
	"github.com/playnux/core/store/builtins/steam"
	"github.com/playnux/core/store/builtins/xbox"
)

const (
	settingLocalFolders = "local.folders"
	settingROMFolders   = "emulation.folders"
)

// StoreInfo é o estado resumido de uma loja para a barra lateral.
type StoreInfo struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Connected   bool                `json:"connected"`
	Clients     []ApplicationStatus `json:"clients"`
	APIKeySaved bool                `json:"api_key_saved"`
}

type ApplicationStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// Core é a fachada usada pelo serviço Wails.
type App struct {
	artworkDir string
	dataDir    string
	db         *database.DB
	manager    *auth.Manager
	providers  map[string]store.StoreProvider
	launcher   *launcher.Launcher
	metadataMu sync.Mutex
	downloadMu sync.Mutex
	libraryMu  sync.RWMutex
	library    LibrarySettings
	metadata   *metadata.Service
	artwork    *metadata.ArtworkManager
	linux      *linux.Provider
	emulation  *emulation.Provider
}

// DefaultDBPath devolve ~/.config/playnux/playnux.db (ou o equivalente XDG).
func DefaultDBPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "playnux", "playnux.db"), nil
}

// DefaultProviders registra todas as lojas suportadas pelo Playnux.
func DefaultProviders() []store.StoreProvider {
	return []store.StoreProvider{
		steam.NewSteamProvider(),
		epic.NewEpicProvider(),
		gog.NewGOGProvider(),
		xbox.NewXboxProvider(),
		linux.NewProvider(),
		emulation.NewProvider(),
	}
}

// New abre o banco e monta o Core com os provedores padrão.
func New(dbPath string, view auth.WebView, sink launcher.Sink) (*App, error) {
	return NewWithProviders(dbPath, view, sink, DefaultProviders()...)
}

// NewWithProviders permite injetar provedores fakes nos testes.
func NewWithProviders(dbPath string, view auth.WebView, sink launcher.Sink, providers ...store.StoreProvider) (*App, error) {
	if dbPath != ":memory:" && !strings.Contains(dbPath, "mode=memory") {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			return nil, fmt.Errorf("criar diretório do banco: %w", err)
		}
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}

	artworkDir := ""
	if dbPath != ":memory:" && !strings.Contains(dbPath, "mode=memory") {
		artworkDir = filepath.Join(filepath.Dir(dbPath), "artwork")
	}
	byID := make(map[string]store.StoreProvider, len(providers))
	core := &App{
		artworkDir: artworkDir,
		dataDir:    filepath.Dir(dbPath),
		db:         db,
		manager:    auth.NewManager(db, view, providers...),
		providers:  byID,
		launcher:   launcher.New(byID, gameproc.NewScanner(), sink),
	}
	for _, provider := range providers {
		byID[provider.ID()] = provider
		if managed, ok := provider.(store.ApplicationProvider); ok {
			managed.SetApplicationCommand(applications.Command)
		}
		switch typed := provider.(type) {
		case *linux.Provider:
			core.linux = typed
		case *emulation.Provider:
			core.emulation = typed
		}
	}
	if err := core.loadFolders(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := core.loadEmulation(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	core.artwork = &metadata.ArtworkManager{
		Dir:        artworkDir,
		DB:         db,
		DownloadMu: &core.downloadMu,
		Search: func(ctx context.Context, query, field string) ([]metadata.WebImage, error) {
			core.metadataMu.Lock()
			provider, ok := core.metadata.Art.(interface {
				SearchImages(context.Context, string, string) ([]steamgriddb.ImageChoice, error)
			})
			core.metadataMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("cadastre a chave em Configurações → SteamGridDB")
			}
			items, err := provider.SearchImages(ctx, query, field)
			if err != nil {
				return nil, err
			}
			results := make([]metadata.WebImage, 0, 24)
			for _, item := range items {
				if len(results) == 24 {
					break
				}
				results = append(results, metadata.WebImage{URL: item.URL, Thumbnail: item.Thumb, Width: item.Width, Height: item.Height})
			}
			return results, nil
		},
		Lookup: func(ctx context.Context, id, field string) (string, error) {
			game, err := core.db.GetGame(ctx, id)
			if err != nil {
				return "", err
			}
			core.metadataMu.Lock()
			provider, ok := core.metadata.Art.(interface {
				LookupImage(context.Context, string, string) (string, error)
			})
			core.metadataMu.Unlock()
			if !ok {
				return "", fmt.Errorf("cadastre a chave em Configurações → SteamGridDB")
			}
			return provider.LookupImage(ctx, game.Name, field)
		},
	}
	if err := core.loadSelection(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := core.loadLibrarySettings(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := core.loadMetadataProviders(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return core, nil
}

func (c *App) Close() error {
	if c == nil {
		return nil
	}
	c.launcher.Close()
	return c.db.Close()
}

func (c *App) ListGames(ctx context.Context, filter string) ([]*models.Game, error) {
	games, err := c.db.ListGames(ctx, filter, 2000)
	if err != nil {
		return nil, err
	}
	games = slices.DeleteFunc(games, linux.IsImportedLibraryTool)
	return c.applyLibraryView(games), nil
}

func (c *App) SearchGames(ctx context.Context, query string) ([]*models.Game, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return c.ListGames(ctx, "all")
	}
	games, err := c.db.SearchGames(ctx, query, 200)
	if err != nil {
		return nil, err
	}
	games = slices.DeleteFunc(games, linux.IsImportedLibraryTool)
	return c.applyLibraryView(games), nil
}

func (c *App) GetGame(ctx context.Context, id string) (*models.Game, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id do jogo vazio")
	}
	game, err := c.db.GetGame(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.withVariants(ctx, game), nil
}

func (c *App) SyncStore(ctx context.Context, storeID string) ([]*models.Game, error) {
	return c.manager.Sync(ctx, storeID)
}

func (c *App) Login(ctx context.Context, storeID string) ([]*models.Game, error) {
	_, games, err := c.manager.Login(ctx, storeID)
	return games, err
}

func (c *App) LaunchGame(ctx context.Context, game *models.Game) error {
	options, err := c.LaunchOptions(ctx, game)
	if err != nil {
		return err
	}
	if err := c.applyCompatibility(game, &options); err != nil {
		return err
	}
	return c.launcher.LaunchWithOptions(game, options)
}

// EnrichGame busca metadados no IGDB/SteamGridDB e os persiste. A UI chama sob
// demanda para não gastar cota nem atrasar a sincronização das lojas.
func (c *App) EnrichGame(ctx context.Context, id string) (*models.Game, error) {
	policy, err := c.MetadataPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return c.EnrichGameUsingPolicy(ctx, id, policy)
}

func (c *App) EnrichGameUsingPolicy(ctx context.Context, id string, policy MetadataPolicy) (*models.Game, error) {
	if err := validateMetadataPolicy(policy); err != nil {
		return nil, err
	}
	c.metadataMu.Lock()
	service := c.metadata
	c.metadataMu.Unlock()

	c.downloadMu.Lock()
	defer c.downloadMu.Unlock()
	game, err := c.db.GetGame(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *game
	var official *models.Game
	if slices.Contains(policy.Sources, metadata.SourceStore) {
		official, err = c.db.StoreMetadata(ctx, game.PluginID, game.GameID)
		if err != nil {
			return nil, err
		}
	}
	if err := service.EnrichPrioritized(ctx, game, official, policy.Sources, policy.Mode); err != nil {
		return nil, err
	}
	c.artwork.CacheProviderArtwork(ctx, before, game)
	if err := c.db.UpdateGameMetadata(ctx, game); err != nil {
		return nil, err
	}
	return c.db.GetGame(ctx, id)
}

// MetadataConfigured informa se há ao menos uma chave de metadados no ambiente.
func (c *App) MetadataConfigured() bool {
	c.metadataMu.Lock()
	defer c.metadataMu.Unlock()
	return c.metadata != nil && c.metadata.Configured()
}

// ImportInstalled importa somente os jogos instalados localmente de uma loja.
func (c *App) ImportInstalled(ctx context.Context, storeID string) ([]*models.Game, error) {
	return c.manager.ImportInstalled(ctx, storeID)
}

// ConnectSteamID conecta a Steam pelo SteamID64 público, sem senha.
func (c *App) ConnectSteamID(ctx context.Context, steamID string) ([]*models.Game, error) {
	return c.manager.ConnectAccount(ctx, "steam", steamID)
}

// DetectSteamID lê o SteamID64 do cliente Steam instalado neste PC.
func (c *App) DetectSteamID(context.Context) (string, error) {
	return steam.DetectLocalAccountID()
}

// AddManualGame cadastra um executável escolhido pelo usuário. O jogo entra na
// biblioteca imediatamente, sem passar por loja, rede ou autenticação.
func (c *App) AddManualGame(ctx context.Context, executable, name string) (*models.Game, error) {
	path, command, args, err := resolveManualExecutable(executable)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
		name = strings.Join(strings.Fields(name), " ")
	}
	if name == "" {
		name = filepath.Base(path)
	}

	info, _ := os.Stat(path)
	var size uint64
	if info != nil {
		size = uint64(info.Size())
	}
	tags := []string{"Manual"}
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		tags = append(tags, "Wine")
	}

	game := &models.Game{
		GameID:           "manual:" + path,
		PluginID:         "linux",
		Source:           "Manual",
		Name:             name,
		SortingName:      name,
		Executable:       command,
		Arguments:        args,
		InstallDirectory: filepath.Dir(path),
		InstallSize:      size,
		IsInstalled:      true,
		Platforms:        []string{"PC (Linux)"},
		Tags:             tags,
	}
	if err := c.db.UpsertGames(ctx, []*models.Game{game}); err != nil {
		return nil, err
	}
	return c.db.GetGame(ctx, game.ID)
}

func resolveManualExecutable(raw string) (path, command string, args []string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil, fmt.Errorf("selecione um executável")
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", "", nil, homeErr
		}
		if raw == "~" {
			raw = home
		} else {
			raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
		}
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		return "", "", nil, fmt.Errorf("executável não encontrado: %s", absolute)
	}

	switch strings.ToLower(filepath.Ext(absolute)) {
	case ".exe":
		wine, lookupErr := exec.LookPath("wine")
		if lookupErr != nil {
			return "", "", nil, fmt.Errorf("para adicionar .exe instale o Wine (comando wine não encontrado)")
		}
		return absolute, wine, []string{absolute}, nil
	case ".sh":
		if info.Mode().Perm()&0o111 != 0 {
			return absolute, absolute, nil, nil
		}
		shell, lookupErr := exec.LookPath("sh")
		if lookupErr != nil {
			return "", "", nil, fmt.Errorf("script .sh sem permissão de execução e sh não encontrado")
		}
		return absolute, shell, []string{absolute}, nil
	}

	if info.Mode().Perm()&0o111 == 0 {
		return "", "", nil, fmt.Errorf("%s não tem permissão de execução (+x)", absolute)
	}
	return absolute, absolute, nil, nil
}

// ── Pastas locais ───────────────────────────────────────────────────────────

// LocalFolders devolve as pastas DRM-free configuradas.
func (c *App) LocalFolders() []string {
	if c.linux == nil {
		return nil
	}
	return c.linux.Folders()
}

// RomFolders devolve as pastas extras de ROMs (além das padrão do EmuDeck).
func (c *App) RomFolders() []string {
	if c.emulation == nil {
		return nil
	}
	return c.emulation.Folders()
}

// AddLocalFolder salva uma pasta DRM-free e sincroniza a fonte local.
func (c *App) AddLocalFolder(ctx context.Context, path string) ([]*models.Game, error) {
	if c.linux == nil {
		return nil, fmt.Errorf("fonte de jogos Linux não registrada")
	}
	return c.addFolder(ctx, settingLocalFolders, path, c.linux.Folders(), c.linux.SetFolders, "linux")
}

// RemoveLocalFolder deixa de escanear uma pasta DRM-free.
func (c *App) RemoveLocalFolder(ctx context.Context, path string) ([]*models.Game, error) {
	if c.linux == nil {
		return nil, fmt.Errorf("fonte de jogos Linux não registrada")
	}
	return c.removeFolder(ctx, settingLocalFolders, path, c.linux.Folders(), c.linux.SetFolders, "linux")
}

// AddRomFolder salva uma pasta extra de ROMs e sincroniza a emulação.
func (c *App) AddRomFolder(ctx context.Context, path string) ([]*models.Game, error) {
	if c.emulation == nil {
		return nil, fmt.Errorf("fonte de emulação não registrada")
	}
	return c.addFolder(ctx, settingROMFolders, path, c.emulation.Folders(), c.emulation.SetFolders, "emulation")
}

// RemoveRomFolder deixa de escanear uma pasta extra de ROMs.
func (c *App) RemoveRomFolder(ctx context.Context, path string) ([]*models.Game, error) {
	if c.emulation == nil {
		return nil, fmt.Errorf("fonte de emulação não registrada")
	}
	return c.removeFolder(ctx, settingROMFolders, path, c.emulation.Folders(), c.emulation.SetFolders, "emulation")
}

func (c *App) addFolder(ctx context.Context, setting, path string, current []string, apply func([]string), storeID string) ([]*models.Game, error) {
	folder, err := normalizeFolder(path)
	if err != nil {
		return nil, err
	}
	for _, existing := range current {
		if existing == folder {
			return c.manager.Sync(ctx, storeID)
		}
	}
	folders := append(append([]string(nil), current...), folder)
	sort.Strings(folders)
	if err := c.saveFolders(ctx, setting, folders); err != nil {
		return nil, err
	}
	apply(folders)
	return c.manager.Sync(ctx, storeID)
}

func (c *App) removeFolder(ctx context.Context, setting, path string, current []string, apply func([]string), storeID string) ([]*models.Game, error) {
	folder, err := normalizeFolderPath(path)
	if err != nil {
		return nil, err
	}
	folders := make([]string, 0, len(current))
	for _, existing := range current {
		if existing != folder {
			folders = append(folders, existing)
		}
	}
	if err := c.saveFolders(ctx, setting, folders); err != nil {
		return nil, err
	}
	apply(folders)
	if storeID == "emulation" && c.emulation != nil {
		mapping, mapErr := c.emulationFolderProfiles(ctx)
		if mapErr != nil {
			return nil, mapErr
		}
		if _, found := mapping[folder]; found {
			delete(mapping, folder)
			if err := c.saveEmulationFolderProfiles(ctx, mapping); err != nil {
				return nil, err
			}
			c.emulation.SetFolderProfiles(mapping)
		}
	}

	// Remove os cartões que pertenciam à pasta antes de sincronizar novamente.
	source, prefix := "", ""
	switch storeID {
	case "linux":
		source, prefix = "Local", "local:"+folder+"/"
	case "emulation":
		source, prefix = "Emulação", "rom:"+folder+"/"
	}
	if prefix != "" {
		if _, err := c.db.DeleteGamesByGameIDPrefix(ctx, source, prefix); err != nil {
			return nil, err
		}
	}
	return c.manager.Sync(ctx, storeID)
}

func (c *App) saveFolders(ctx context.Context, key string, folders []string) error {
	raw, err := json.Marshal(folders)
	if err != nil {
		return err
	}
	return c.db.SetSetting(ctx, key, string(raw))
}

func (c *App) loadFolders(ctx context.Context) error {
	if c.linux != nil {
		folders, err := c.settingFolders(ctx, settingLocalFolders)
		if err != nil {
			return err
		}
		c.linux.SetFolders(folders)
	}
	if c.emulation != nil {
		folders, err := c.settingFolders(ctx, settingROMFolders)
		if err != nil {
			return err
		}
		c.emulation.SetFolders(folders)
	}
	return nil
}

func (c *App) settingFolders(ctx context.Context, key string) ([]string, error) {
	raw, err := c.db.GetSetting(ctx, key)
	if errors.Is(err, database.ErrSettingNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var folders []string
	if err := json.Unmarshal([]byte(raw), &folders); err != nil {
		// Preferência corrompida não deve impedir o app de abrir.
		return nil, nil
	}
	return folders, nil
}

func normalizeFolder(path string) (string, error) {
	absolute, err := normalizeFolderPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("pasta não encontrada: %s", absolute)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s não é uma pasta", absolute)
	}
	return absolute, nil
}

func normalizeFolderPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("informe o caminho de uma pasta")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

// Stores devolve as lojas registradas e se há uma sessão persistida. A leitura
// é local (sem rede) para a UI abrir instantaneamente.
func (c *App) Stores(ctx context.Context) []StoreInfo {
	stores := make([]StoreInfo, 0, len(c.providers))
	clientStatus := make(map[store.ApplicationID]ApplicationStatus)
	for id, provider := range c.providers {
		// Fontes locais não têm sessão; as demais aparecem desconectadas quando
		// não há login persistido, sem impedir uma nova tentativa.
		info := StoreInfo{ID: id, Name: provider.Name()}
		if local, ok := provider.(store.LocalProvider); ok && local.IsLocal() {
			info.Connected = true
		} else if session, err := c.db.LoadSession(ctx, id); err == nil {
			info.Connected = true
			info.APIKeySaved = session.APIKey != ""
		}
		if managed, ok := provider.(store.ApplicationProvider); ok {
			for _, required := range managed.ApplicationRequirements() {
				status, found := clientStatus[required]
				if !found {
					_, _, installed := applications.Command(required)
					status = ApplicationStatus{Name: applications.Name(required), Installed: installed}
					clientStatus[required] = status
				}
				info.Clients = append(info.Clients, status)
			}
		}
		stores = append(stores, info)
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].Name < stores[j].Name })
	return stores
}
