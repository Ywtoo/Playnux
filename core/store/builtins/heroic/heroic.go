// Package heroic lê o estado local do Heroic Games Launcher e do Legendary para
// descobrir jogos Epic/GOG instalados no Linux, sem depender de clientes oficiais.
package heroic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Image é uma imagem de catálogo do Heroic/legendary ({type, url}).
type Image struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Install descreve uma instalação local gerenciada pelo Heroic.
type Install struct {
	Path       string // pasta de instalação
	Executable string // relativo à pasta (ou absoluto)
	Size       uint64 // bytes
	Platform   string // "Windows" ou "Linux"
	WineBinary string
	WineName   string
	WinePrefix string
}

// ExecutablePath devolve o caminho completo do executável, se conhecido.
func (i *Install) ExecutablePath() string {
	if i == nil || i.Executable == "" {
		return ""
	}
	if filepath.IsAbs(i.Executable) {
		return i.Executable
	}
	return filepath.Join(i.Path, i.Executable)
}

// Entry é um jogo conhecido pelo Heroic (instalado ou apenas em cache).
type Entry struct {
	AppName string
	Title   string
	Runner  string // loja no Heroic: "legendary", "gog", ...
	Images  []Image
	Install *Install
}

// Installed indica se o jogo está presente no disco.
func (e *Entry) Installed() bool {
	return e != nil && e.Install != nil && e.Install.Path != ""
}

// Cover devolve a melhor capa vertical disponível.
func (e *Entry) Cover() string {
	if e == nil {
		return ""
	}
	if url := ImageURL(e.Images,
		"DieselGameBoxTall", "OfferImageTall", "vertical_cover", "boxArt", "Thumbnail"); url != "" {
		return url
	}
	if len(e.Images) > 0 {
		return NormalizeURL(e.Images[0].URL)
	}
	return ""
}

// Background devolve a melhor imagem horizontal disponível.
func (e *Entry) Background() string {
	if e == nil {
		return ""
	}
	return ImageURL(e.Images,
		"DieselGameBox", "DieselStoreFrontWide", "OfferImageWide", "horizontal_banner", "background")
}

// ImageURL procura a primeira imagem cujo tipo case com a lista de preferência.
func ImageURL(images []Image, preferred ...string) string {
	for _, want := range preferred {
		for _, img := range images {
			if strings.EqualFold(img.Type, want) && img.URL != "" {
				return NormalizeURL(img.URL)
			}
		}
	}
	return ""
}

// LaunchURI identifies an installed game for the Heroic client.
func LaunchURI(runner, appName string) string {
	return fmt.Sprintf("heroic://launch/%s/%s", runner, appName)
}

// InstallFor devolve a instalação configurada no Heroic para um app_name.
func InstallFor(appName string) *Install {
	for _, dir := range configDirs() {
		if config, ok := readGamesConfig(dir)[appName]; ok && config.Install != nil && config.Install.Path != "" {
			return config.Install
		}
	}
	return nil
}

// EpicLibrary lê o cache do Heroic/Legendary e as instalações do Legendary.
func EpicLibrary() []*Entry {
	entries := loadStore("legendary_library.json", "legendary")
	mergeLegendaryInstalled(entries)
	return sortedEntries(entries)
}

// GOGLibrary lê o cache de biblioteca GOG do Heroic (gogdl).
func GOGLibrary() []*Entry {
	return sortedEntries(loadStore("gog_library.json", "gog"))
}

// ── Leitura dos arquivos locais ─────────────────────────────────────────────

func configDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".config/heroic"),
		filepath.Join(home, ".var/app/com.heroicgameslauncher.hgl/config/heroic"),
	}
}

func loadStore(cacheFile, storeRunner string) map[string]*Entry {
	entries := make(map[string]*Entry)

	for _, dir := range configDirs() {
		for _, entry := range readLibrary(filepath.Join(dir, "store_cache", cacheFile)) {
			if entry.AppName == "" {
				continue
			}
			if existing, ok := entries[entry.AppName]; ok {
				mergeEntry(existing, entry)
				continue
			}
			entries[entry.AppName] = entry
		}
	}

	// GamesConfig guarda o install_path real usado pelo Heroic (o cache da loja
	// pode estar desatualizado). A mesma pasta é compartilhada por todas as
	// lojas, então só adiciona jogos cujo runner pertence a esta biblioteca.
	for _, dir := range configDirs() {
		for appName, config := range readGamesConfig(dir) {
			entry, known := entries[appName]
			if !known {
				if !strings.EqualFold(config.Runner, storeRunner) {
					continue
				}
				entry = &Entry{AppName: appName, Title: appName, Runner: config.Runner}
				entries[appName] = entry
			}
			if entry.Install == nil || entry.Install.Path == "" {
				entry.Install = config.Install
			}
		}
	}

	return entries
}

func mergeEntry(dst, src *Entry) {
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if len(dst.Images) == 0 {
		dst.Images = src.Images
	}
	if !dst.Installed() && src.Installed() {
		dst.Install = src.Install
	}
}

type rawInstall struct {
	InstallPath string     `json:"install_path"`
	Executable  string     `json:"executable"`
	InstallSize flexUint64 `json:"install_size"`
	Platform    string     `json:"platform"`
}

type rawEntry struct {
	AppName  string      `json:"app_name"`
	Title    string      `json:"title"`
	AppTitle string      `json:"app_title"`
	Runner   string      `json:"runner"`
	Install  *rawInstall `json:"install"`
	Images   []Image     `json:"images"`
	Extra    *struct {
		Images []Image `json:"images"`
	} `json:"extra"`
	Metadata *struct {
		Title     string  `json:"title"`
		KeyImages []Image `json:"keyImages"`
	} `json:"metadata"`
}

func readLibrary(path string) []*Entry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var list []rawEntry
	if err := json.Unmarshal(data, &list); err != nil {
		// Alguns caches antigos são mapas app_name -> GameInfo.
		byName := make(map[string]rawEntry)
		if err := json.Unmarshal(data, &byName); err != nil {
			return nil
		}
		for name, raw := range byName {
			if raw.AppName == "" {
				raw.AppName = name
			}
			list = append(list, raw)
		}
	}

	entries := make([]*Entry, 0, len(list))
	for _, raw := range list {
		entry := raw.toEntry()
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (r rawEntry) toEntry() *Entry {
	appName := strings.TrimSpace(r.AppName)
	if appName == "" {
		return nil
	}

	title := r.Title
	if r.AppTitle != "" && (title == "" || title == appName) {
		title = r.AppTitle
	}
	if title == "" && r.Metadata != nil {
		title = r.Metadata.Title
	}
	if title == "" {
		title = appName
	}

	images := r.Images
	if len(images) == 0 && r.Extra != nil {
		images = r.Extra.Images
	}
	if len(images) == 0 && r.Metadata != nil {
		images = r.Metadata.KeyImages
	}

	entry := &Entry{AppName: appName, Title: title, Runner: r.Runner, Images: images}
	if r.Install != nil && r.Install.InstallPath != "" {
		entry.Install = &Install{
			Path:       r.Install.InstallPath,
			Executable: r.Install.Executable,
			Size:       uint64(r.Install.InstallSize),
			Platform:   r.Install.Platform,
		}
	}
	return entry
}

type rawGameConfig struct {
	AppName     string `json:"app_name"`
	Title       string `json:"title"`
	Runner      string `json:"runner"`
	WinePrefix  string `json:"winePrefix"`
	WineVersion *struct {
		Bin  string `json:"bin"`
		Name string `json:"name"`
	} `json:"wineVersion"`
	Install *rawInstall `json:"install"`
}

type gameConfig struct {
	Runner  string
	Install *Install
}

func readGamesConfig(heroicDir string) map[string]*gameConfig {
	result := make(map[string]*gameConfig)

	files, err := os.ReadDir(filepath.Join(heroicDir, "GamesConfig"))
	if err != nil {
		return result
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(heroicDir, "GamesConfig", file.Name()))
		if err != nil {
			continue
		}

		var cfg rawGameConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		appName := cfg.AppName
		if appName == "" {
			appName = strings.TrimSuffix(file.Name(), ".json")
		}
		if cfg.Install == nil || cfg.Install.InstallPath == "" {
			continue
		}

		install := &Install{
			Path:       cfg.Install.InstallPath,
			Executable: cfg.Install.Executable,
			Size:       uint64(cfg.Install.InstallSize),
			Platform:   cfg.Install.Platform,
			WinePrefix: cfg.WinePrefix,
		}
		if cfg.WineVersion != nil {
			install.WineBinary = cfg.WineVersion.Bin
			install.WineName = cfg.WineVersion.Name
		}
		result[appName] = &gameConfig{Runner: cfg.Runner, Install: install}
	}
	return result
}

func mergeLegendaryInstalled(entries map[string]*Entry) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	paths := []string{
		filepath.Join(home, ".config/legendary/installed.json"),
		filepath.Join(home, ".var/app/com.heroicgameslauncher.hgl/config/legendary/installed.json"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var installed map[string]struct {
			Title       string     `json:"title"`
			InstallPath string     `json:"install_path"`
			Executable  string     `json:"executable"`
			InstallSize flexUint64 `json:"install_size"`
		}
		if err := json.Unmarshal(data, &installed); err != nil {
			continue
		}

		for appName, raw := range installed {
			if raw.InstallPath == "" {
				continue
			}
			entry, ok := entries[appName]
			if !ok {
				title := raw.Title
				if title == "" {
					title = appName
				}
				entry = &Entry{AppName: appName, Title: title}
				entries[appName] = entry
			}
			if !entry.Installed() {
				entry.Install = &Install{
					Path:       raw.InstallPath,
					Executable: raw.Executable,
					Size:       uint64(raw.InstallSize),
					Platform:   "Windows",
				}
			}
		}
	}
}

func sortedEntries(entries map[string]*Entry) []*Entry {
	result := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	return result
}

// NormalizeURL completa URLs protocol-relative ("//cdn/...") devolvidas pela GOG.
func NormalizeURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

// flexUint64 aceita números ou strings (o Heroic salva install_size como string).
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	value = strings.Trim(value, `"`)
	if value == "" || value == "null" {
		*f = 0
		return nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return err
	}
	*f = flexUint64(parsed)
	return nil
}
