// Package host implementa o serviço Wails do Playnux.
//
// A regra de negócio fica em core/app; este pacote só traduz as
// chamadas do frontend, abre a janela de login e aplica o Deep Sleep visual e
// de memória quando o launcher detecta o processo do jogo.
package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/playnux/core/app"
	"github.com/playnux/core/auth"
	"github.com/playnux/core/metadata"
	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/runner/wine"
	"github.com/playnux/core/store/builtins/emulation"
	"github.com/playnux/ui/internal/loginbridge"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	eventGameplaySleep = "gameplay:sleep"
	eventGameplayWake  = "gameplay:wake"
)

// Host é o serviço registrado no Wails e a ponte de login do Core.
type Host struct {
	core *app.App

	mu     sync.RWMutex
	app    *application.App
	window *application.WebviewWindow
	ctx    context.Context

	sleeping atomic.Bool
}

func New(core *app.App) *Host {
	return &Host{core: core}
}

// SetCore completa a ligação depois que o Core foi criado com o Host como
// WebView e sink de gameplay (evita um ciclo de construção). Fica fora do
// serviço para não virar binding utilizável pelo frontend.
func SetCore(h *Host, core *app.App) {
	h.core = core
}

// ServiceName melhora o nome exibido nos logs do Wails.
func (h *Host) ServiceName() string { return "Playnux" }

// Attach liga o serviço à aplicação e à janela principal já criadas. É uma
// função de pacote (não um método) para não ser exposta como binding.
func Attach(h *Host, app *application.App, window *application.WebviewWindow) {
	h.mu.Lock()
	h.app = app
	h.window = window
	h.mu.Unlock()
}

func (h *Host) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	h.mu.Lock()
	h.ctx = ctx
	h.mu.Unlock()
	return nil
}

func (h *Host) ServiceShutdown() error {
	if h.core == nil {
		return nil
	}
	return h.core.Close()
}

func (h *Host) appContext() context.Context {
	h.mu.RLock()
	ctx := h.ctx
	h.mu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// ── Biblioteca ───────────────────────────────────────────────────────────────

func (h *Host) GetGames(filter string) ([]*models.Game, error) {
	return h.core.ListGames(h.appContext(), filter)
}

func (h *Host) SearchGames(query string) ([]*models.Game, error) {
	return h.core.SearchGames(h.appContext(), query)
}

func (h *Host) GetGame(gameID string) (*models.Game, error) {
	return h.core.GetGame(h.appContext(), gameID)
}

func (h *Host) GetStores() []app.StoreInfo {
	return h.core.Stores(h.appContext())
}

func (h *Host) SyncStore(storeID string) ([]*models.Game, error) {
	return h.core.SyncStore(h.appContext(), storeID)
}

// EnrichGame consulta IGDB/SteamGridDB e devolve o jogo atualizado com
// sinopse, empresas, nota e trailer.
func (h *Host) EnrichGame(gameID string) (*models.Game, error) {
	return h.core.EnrichGame(h.appContext(), gameID)
}

func (h *Host) MetadataConfigured() bool {
	return h.core.MetadataConfigured()
}

// ImportInstalledGames importa somente a biblioteca local da loja (Steam ACF,
// Heroic/Legendary) sem exigir login web.
func (h *Host) ImportInstalledGames(storeID string) ([]*models.Game, error) {
	return h.core.ImportInstalled(h.appContext(), storeID)
}

// ConnectSteamID conecta a Steam pelo SteamID64 e importa os jogos locais.
func (h *Host) ConnectSteamID(steamID string) ([]*models.Game, error) {
	return h.core.ConnectSteamID(h.appContext(), steamID)
}

func (h *Host) DetectSteamID() (string, error) {
	return h.core.DetectSteamID(h.appContext())
}

// AddManualGame cadastra um executável local escolhido pelo usuário.
func (h *Host) AddManualGame(executable, name string) (*models.Game, error) {
	return h.core.AddManualGame(h.appContext(), executable, name)
}

// LocalApps lista os aplicativos de jogos detectados no sistema para o
// usuário escolher quais adicionar.
func (h *Host) LocalApps() ([]app.LocalApp, error) {
	return h.core.LocalApps(h.appContext()), nil
}

// AddLocalApps adiciona somente os aplicativos selecionados.
func (h *Host) AddLocalApps(ids []string) ([]*models.Game, error) {
	return h.core.AddLocalApps(h.appContext(), ids)
}

// ClearLibrary apaga todos os jogos e fontes locais configuradas.
func (h *Host) ClearLibrary() error {
	return h.core.ClearLibrary(h.appContext())
}

func (h *Host) GetUISettings() (app.UISettings, error) {
	return h.core.UISettings(h.appContext())
}

func (h *Host) SaveUISettings(settings app.UISettings) error {
	return h.core.SaveUISettings(h.appContext(), settings)
}

func (h *Host) GetLibrarySettings() (app.LibrarySettings, error) {
	return h.core.LibrarySettings(h.appContext())
}

func (h *Host) SaveLibrarySettings(settings app.LibrarySettings) error {
	return h.core.SaveLibrarySettings(h.appContext(), settings)
}

// CopyGameArtwork usa as imagens de outra cópia/loja do jogo unificado.
func (h *Host) CopyGameArtwork(gameID, sourceID string) (*models.Game, error) {
	return h.core.CopyGameArtwork(h.appContext(), gameID, sourceID)
}

// PickGameExecutable abre o seletor nativo de arquivos do Wails/GTK.
func (h *Host) PickGameExecutable() (string, error) {
	h.mu.RLock()
	app := h.app
	window := h.window
	h.mu.RUnlock()
	if app == nil {
		return "", fmt.Errorf("janela Wails ainda não inicializada")
	}

	dialog := app.Dialog.OpenFile()
	dialog.SetTitle("Selecionar executável do jogo")
	dialog.AddFilter("Executáveis", "*.sh;*.x86_64;*.x86;*.AppImage;*.exe")
	dialog.AddFilter("Todos os arquivos", "*")
	if window != nil {
		dialog.AttachToWindow(window)
	}
	return dialog.PromptForSingleSelection()
}

// ── Pastas locais ────────────────────────────────────────────────────────────

func (h *Host) GetLocalFolders() []string {
	return h.core.LocalFolders()
}

func (h *Host) GetRomFolders() []string {
	return h.core.RomFolders()
}

func (h *Host) AddLocalFolder(path string) ([]*models.Game, error) {
	return h.core.AddLocalFolder(h.appContext(), path)
}

func (h *Host) RemoveLocalFolder(path string) ([]*models.Game, error) {
	return h.core.RemoveLocalFolder(h.appContext(), path)
}

func (h *Host) AddRomFolder(path string) ([]*models.Game, error) {
	return h.core.AddRomFolder(h.appContext(), path)
}

func (h *Host) RemoveRomFolder(path string) ([]*models.Game, error) {
	return h.core.RemoveRomFolder(h.appContext(), path)
}

// GetEmulators lists built-in/detected emulators and their launch profiles.
func (h *Host) GetEmulators() []emulation.EmulatorDefinition {
	return h.core.Emulators(h.appContext())
}

// GetEmulationFolderProfiles returns the chosen profile for each ROM folder.
func (h *Host) GetEmulationFolderProfiles() map[string]string {
	return h.core.EmulationFolderProfiles()
}

// SaveEmulator creates or updates a user-defined emulator and its profiles.
func (h *Host) SaveEmulator(input emulation.CustomEmulator) error {
	return h.core.SaveEmulator(h.appContext(), input)
}

func (h *Host) RemoveEmulator(id string) error {
	return h.core.RemoveEmulator(h.appContext(), id)
}

// AddEmulationFolder scans a folder with a specific emulator profile.
func (h *Host) AddEmulationFolder(path, profileID string) ([]*models.Game, error) {
	return h.core.AddEmulationFolder(h.appContext(), path, profileID)
}

// LoginStore abre a WebView oficial e, ao concluir, salva a sessão e importa a
// biblioteca. É uma chamada longa: o frontend mostra o estado "conectando".
func (h *Host) LoginStore(storeID string) ([]*models.Game, error) {
	return h.core.Login(h.appContext(), storeID)
}

// LaunchGame inicia o jogo e deixa o monitor de processo em background. O Deep
// Sleep é acionado depois que o processo do jogo é encontrado.
func (h *Host) LaunchGame(gameID string) error {
	game, err := h.core.GetGame(h.appContext(), gameID)
	if err != nil {
		return err
	}
	return h.core.LaunchGame(h.appContext(), game)
}

// ── Login por WebView ────────────────────────────────────────────────────────

// loginReport é uma leitura enviada pelo script da página remota.
type loginReport = loginbridge.Report

// OpenLogin implementa auth.WebView. Abre uma janela limpa, injeta um script
// que observa URL/cookies/corpo e recebe o resultado num servidor loopback
// efêmero (sem expor portas fixas).
func (h *Host) OpenLogin(ctx context.Context, req auth.LoginRequest) (auth.LoginResult, error) {
	if req.URL == "" {
		return auth.LoginResult{}, fmt.Errorf("URL de login vazia")
	}
	h.mu.RLock()
	app := h.app
	h.mu.RUnlock()
	if app == nil {
		return auth.LoginResult{}, fmt.Errorf("janela Wails ainda não inicializada")
	}

	token, err := randomToken()
	if err != nil {
		return auth.LoginResult{}, err
	}

	reports := make(chan loginReport, 4)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return auth.LoginResult{}, fmt.Errorf("servidor de login: %w", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           reportHandler(token, reports),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	reportURL := fmt.Sprintf("http://%s/report", listener.Addr().String())
	script := loginbridge.ReportScript(loginbridge.Options{
		Token:       token,
		ReportURL:   reportURL,
		RedirectURL: req.RedirectURLPrefix,
		Cookies:     req.CookieNames,
	})

	width, height, minWidth, minHeight := FitWindowToScreen(app, 980, 720, 480, 600)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "playnux-login-" + token,
		Title:            "Playnux — login " + req.PluginID,
		Width:            width,
		Height:           height,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		URL:              req.URL,
		JS:               script,
		BackgroundColour: application.NewRGB(11, 13, 18),
	})
	// Em alguns ambientes GTK3 a janela secundária é criada oculta. Exibir e
	// focar explicitamente evita que o login pareça não ter aberto.
	window.Show()
	window.Center()
	window.Focus()
	log.Printf("login %s: janela aberta", req.PluginID)

	closed := make(chan struct{})
	var closeOnce sync.Once
	markClosed := func() { closeOnce.Do(func() { close(closed) }) }
	unregister := window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		markClosed()
	})
	defer unregister()
	defer markClosed()
	defer window.Close()

	timeout := time.NewTimer(15 * time.Minute)
	defer timeout.Stop()

	for {
		select {
		case report := <-reports:
			return auth.LoginResult{
				RedirectURL: report.URL,
				Body:        report.Body,
				Cookies:     report.Cookies,
			}, nil
		case <-closed:
			return auth.LoginResult{}, fmt.Errorf("login cancelado: a janela foi fechada")
		case <-ctx.Done():
			return auth.LoginResult{}, fmt.Errorf("login cancelado: %w", ctx.Err())
		case <-timeout.C:
			return auth.LoginResult{}, fmt.Errorf("login expirou após 15 minutos")
		}
	}
}

func reportHandler(token string, reports chan<- loginReport) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != token {
			http.Error(w, "token inválido", http.StatusForbidden)
			return
		}
		report, err := loginbridge.DecodeReport(r.URL.Query().Get("d"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case reports <- report:
		default:
			// A janela já recebeu um relatório; não bloqueia o servidor.
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html lang="pt-BR"><meta charset="utf-8">` +
			`<title>Playnux</title><body style="font-family:system-ui;background:#0b0d12;color:#e8eaf0;` +
			`display:grid;place-items:center;height:100vh;margin:0"><p>Login concluído. ` +
			`Você já pode fechar esta janela.</p></body></html>`))
	})
	return mux
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("gerar token de login: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ── Deep Sleep ───────────────────────────────────────────────────────────────

// Sleep congela a UI, libera as capas da memória e devolve o heap ao sistema.
// A janela só é escondida depois que o script visual teve chance de rodar.
func (h *Host) Sleep(game *models.Game) {
	if game == nil || !h.sleeping.CompareAndSwap(false, true) {
		return
	}

	h.mu.RLock()
	app := h.app
	window := h.window
	h.mu.RUnlock()

	if app != nil {
		app.Event.Emit(eventGameplaySleep, game)
	}
	if window != nil {
		window.ExecJS("window.playnuxSleep&&window.playnuxSleep()")
		time.AfterFunc(150*time.Millisecond, func() {
			if h.sleeping.Load() {
				window.Hide()
			}
		})
	}

	runtime.GC()
	debug.FreeOSMemory()
	log.Printf("deep sleep: %s", game.Name)
}

// Wake restaura a janela e retoma a renderização o quanto antes.
func (h *Host) Wake(game *models.Game) {
	if game == nil || !h.sleeping.CompareAndSwap(true, false) {
		return
	}

	h.mu.RLock()
	app := h.app
	window := h.window
	h.mu.RUnlock()

	if window != nil {
		window.Show()
		window.Focus()
		window.ExecJS("window.playnuxWake&&window.playnuxWake()")
	}
	if app != nil {
		app.Event.Emit(eventGameplayWake, game)
	}
	log.Printf("deep sleep encerrado: %s", game.Name)
}

func (h *Host) GetMetadataSettings() (app.MetadataSettings, error) {
	return h.core.MetadataSettings(h.appContext())
}

func (h *Host) SaveMetadataPolicy(automatic bool, sources []string, mode string) error {
	return h.core.SaveMetadataPolicy(h.appContext(), app.MetadataPolicy{Automatic: automatic, Sources: sources, Mode: mode})
}

func (h *Host) SetMetadataCredentials(id, clientID, key string) error {
	return h.core.SetMetadataCredentials(h.appContext(), id, clientID, key)
}

func (h *Host) SaveGameArtwork(gameID, field, encoded string) error {
	return h.core.SaveGameArtwork(h.appContext(), gameID, field, encoded)
}

func (h *Host) ConfigureSteam(accountID, key string, clearKey bool) error {
	return h.core.ConfigureSteam(h.appContext(), accountID, key, clearKey)
}

func (h *Host) SearchGoogleImages(query string, transparent bool) ([]metadata.WebImage, error) {
	return h.searchGoogleImages(h.appContext(), query, transparent)
}

func (h *Host) ApplyWebImage(gameID, field string, image metadata.WebImage) error {
	return h.core.ApplyWebImage(h.appContext(), gameID, field, image)
}

func (h *Host) GetLaunchOptions(gameID string) (launcher.Options, error) {
	ctx := h.appContext()
	game, err := h.core.GetGame(ctx, gameID)
	if err != nil {
		return launcher.Options{}, err
	}
	return h.core.LaunchOptions(ctx, game)
}

func (h *Host) SaveLaunchOptions(gameID, directory string, arguments []string, runner, protonPath string) error {
	return h.core.SaveLaunchOptions(h.appContext(), gameID, directory, arguments, runner, protonPath)
}

func (h *Host) GetCompatibilityRunners() []wine.Runner {
	return h.core.CompatibilityRunners(h.appContext())
}

// GetWindowsExecutable reports the .exe resolved for a game so the properties
// window can enable the isolated Proton selector only when it is usable.
func (h *Host) GetWindowsExecutable(gameID string) (string, error) {
	return h.core.WindowsExecutable(h.appContext(), gameID)
}

func (h *Host) SaveGameDetails(gameID string, edit app.GameEdit) (*models.Game, error) {
	return h.core.SaveGameDetails(h.appContext(), gameID, edit)
}

func (h *Host) ApplyProviderArtwork(gameID, field string) error {
	return h.core.ApplyProviderArtwork(h.appContext(), gameID, field)
}

func (h *Host) DownloadGameMetadata(gameID string, sources []string, mode string) (*models.Game, error) {
	policy := app.MetadataPolicy{Sources: sources, Mode: mode}
	return h.core.EnrichGameUsingPolicy(h.appContext(), gameID, policy)
}

func (h *Host) SearchProviderArtwork(query, field string) ([]metadata.WebImage, error) {
	return h.core.SearchProviderArtwork(h.appContext(), query, field)
}

func (h *Host) ApplySteamGridImage(gameID, field string, image metadata.WebImage) error {
	return h.core.ApplySteamGridImage(h.appContext(), gameID, field, image)
}

func (h *Host) GetGameRuntime(gameID string) (app.GameRuntime, error) {
	return h.core.GameRuntime(h.appContext(), gameID)
}

// GetGameCompatibility returns ProtonDB recommendations and anti-cheat status
// for the details banner.
func (h *Host) GetGameCompatibility(gameID string) (app.GameCompatibility, error) {
	return h.core.GameCompatibility(h.appContext(), gameID)
}
