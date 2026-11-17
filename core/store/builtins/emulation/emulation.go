// Package emulation detecta emuladores instalados e monta a biblioteca a
// partir de pastas de ROMs no estilo EmuDeck/EmulationStation.
package emulation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

// Emulator descreve um emulador instalado nativamente ou via Flatpak.
type Emulator struct {
	ID        string
	Name      string
	Binary    string
	FlatpakID string
}

// Command devolve o executável e os argumentos para abrir uma ROM.
func (e Emulator) Command(rom string) (string, []string) {
	if e.Binary != "" {
		return e.Binary, []string{rom}
	}
	if e.FlatpakID != "" {
		return "flatpak", []string{"run", e.FlatpakID, rom}
	}
	return "", nil
}

// System associa uma pasta de ROMs aos emuladores que a suportam.
type System struct {
	Key        string
	Name       string
	Extensions []string
	Emulators  []string // em ordem de preferência
}

var systems = []System{
	{Key: "ps2", Name: "PlayStation 2", Extensions: []string{".iso", ".chd", ".bin", ".cue", ".zso"}, Emulators: []string{"pcsx2", "retroarch"}},
	{Key: "ps3", Name: "PlayStation 3", Extensions: []string{".iso", ".pkg", ".dec"}, Emulators: []string{"rpcs3"}},
	{Key: "switch", Name: "Nintendo Switch", Extensions: []string{".nsp", ".xci", ".nro"}, Emulators: []string{"ryujinx"}},
	{Key: "gamecube", Name: "Nintendo GameCube", Extensions: []string{".iso", ".gcm", ".rvz"}, Emulators: []string{"dolphin", "retroarch"}},
	{Key: "wii", Name: "Nintendo Wii", Extensions: []string{".iso", ".wbfs", ".rvz"}, Emulators: []string{"dolphin", "retroarch"}},
	{Key: "psp", Name: "PlayStation Portable", Extensions: []string{".iso", ".cso", ".chd"}, Emulators: []string{"ppsspp"}},
	{Key: "ps1", Name: "PlayStation", Extensions: []string{".bin", ".cue", ".chd", ".iso", ".pbp", ".m3u"}, Emulators: []string{"duckstation", "retroarch"}},
	{Key: "snes", Name: "Super Nintendo", Extensions: []string{".sfc", ".smc", ".fig", ".swc"}, Emulators: []string{"retroarch"}},
	{Key: "nes", Name: "Nintendo Entertainment System", Extensions: []string{".nes", ".fds", ".unf"}, Emulators: []string{"retroarch"}},
	{Key: "gba", Name: "Game Boy Advance", Extensions: []string{".gba"}, Emulators: []string{"retroarch"}},
	{Key: "gb", Name: "Game Boy", Extensions: []string{".gb", ".gbc"}, Emulators: []string{"retroarch"}},
	{Key: "n64", Name: "Nintendo 64", Extensions: []string{".n64", ".z64", ".v64"}, Emulators: []string{"retroarch"}},
	{Key: "nds", Name: "Nintendo DS", Extensions: []string{".nds"}, Emulators: []string{"retroarch"}},
	{Key: "genesis", Name: "Mega Drive / Genesis", Extensions: []string{".md", ".gen", ".smd"}, Emulators: []string{"retroarch"}},
	{Key: "dreamcast", Name: "Dreamcast", Extensions: []string{".gdi", ".cdi", ".chd"}, Emulators: []string{"retroarch"}},
	{Key: "saturn", Name: "Sega Saturn", Extensions: []string{".chd", ".cue", ".bin"}, Emulators: []string{"retroarch"}},
	{Key: "arcade", Name: "Arcade", Extensions: []string{".zip", ".7z"}, Emulators: []string{"retroarch"}},
}

var aliases = map[string]string{
	"ps2": "ps2", "playstation2": "ps2",
	"ps3": "ps3", "playstation3": "ps3",
	"switch": "switch", "yuzu": "switch",
	"gamecube": "gamecube", "gc": "gamecube", "ngc": "gamecube",
	"wii": "wii",
	"psp": "psp", "playstationportable": "psp",
	"ps1": "ps1", "psx": "ps1", "playstation": "ps1",
	"snes": "snes", "supernintendo": "snes",
	"nes": "nes", "famicom": "nes",
	"gba": "gba", "gameboyadvance": "gba",
	"gb": "gb", "gbc": "gb", "gameboy": "gb",
	"n64": "n64", "nintendo64": "n64",
	"nds": "nds", "ds": "nds",
	"genesis": "genesis", "megadrive": "genesis", "mega-drive": "genesis",
	"dreamcast": "dreamcast",
	"saturn":    "saturn",
	"arcade":    "arcade", "mame": "arcade", "fbneo": "arcade",
}

type emulatorDef struct {
	Emulator
	Binaries   []string
	FlatpakIDs []string
}

var emulatorDefs = []emulatorDef{
	{Emulator: Emulator{ID: "retroarch", Name: "RetroArch"}, Binaries: []string{"retroarch"}, FlatpakIDs: []string{"org.libretro.RetroArch"}},
	{Emulator: Emulator{ID: "pcsx2", Name: "PCSX2"}, Binaries: []string{"pcsx2", "PCSX2"}, FlatpakIDs: []string{"net.pcsx2.PCSX2"}},
	{Emulator: Emulator{ID: "rpcs3", Name: "RPCS3"}, Binaries: []string{"rpcs3"}, FlatpakIDs: []string{"net.rpcs3.RPCS3"}},
	{Emulator: Emulator{ID: "dolphin", Name: "Dolphin"}, Binaries: []string{"dolphin-emu", "dolphin"}, FlatpakIDs: []string{"org.DolphinEmu.dolphin-emu"}},
	{Emulator: Emulator{ID: "duckstation", Name: "DuckStation"}, Binaries: []string{"duckstation-qt", "duckstation"}, FlatpakIDs: []string{"org.duckstation.DuckStation"}},
	{Emulator: Emulator{ID: "ryujinx", Name: "Ryujinx"}, Binaries: []string{"ryujinx", "Ryujinx"}, FlatpakIDs: []string{"io.github.ryujinx.Ryujinx", "org.ryujinx.Ryujinx"}},
	{Emulator: Emulator{ID: "ppsspp", Name: "PPSSPP"}, Binaries: []string{"ppsspp", "PPSSPPSDL", "PPSSPPQt"}, FlatpakIDs: []string{"org.ppsspp.PPSSPP"}},
}

// IsEmulatorApplication identifies installed emulator applications, separately from ROMs.
func IsEmulatorApplication(id string) bool {
	for _, emulator := range emulatorDefs {
		for _, names := range [][]string{emulator.Binaries, emulator.FlatpakIDs} {
			for _, name := range names {
				if strings.EqualFold(id, name) {
					return true
				}
			}
		}
	}
	return false
}

// Detector pode ser substituído em testes para não depender do host.
type Detector struct {
	LookPath      func(string) (string, error)
	FlatpakInfo   func(string) bool
	checkFlatpaks bool
}

func defaultDetector() Detector {
	return Detector{
		LookPath: exec.LookPath,
		FlatpakInfo: func(id string) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return exec.CommandContext(ctx, "flatpak", "info", id).Run() == nil
		},
		checkFlatpaks: true,
	}
}

// Detect devolve os emuladores disponíveis indexados pelo ID lógico.
func (d Detector) Detect() map[string]Emulator {
	available := make(map[string]Emulator)
	for _, def := range emulatorDefs {
		emulator := def.Emulator
		for _, binary := range def.Binaries {
			if path, err := d.LookPath(binary); err == nil && path != "" {
				emulator.Binary = path
				available[emulator.ID] = emulator
				break
			}
		}
		if emulator.Binary != "" || !d.checkFlatpaks || d.FlatpakInfo == nil {
			continue
		}
		for _, id := range def.FlatpakIDs {
			if d.FlatpakInfo(id) {
				emulator.FlatpakID = id
				available[emulator.ID] = emulator
				break
			}
		}
	}
	return available
}

// Provider é a fonte local de jogos de emulação.
type Provider struct {
	mu       sync.RWMutex
	folders  []string
	detector Detector
}

func NewProvider() *Provider {
	return &Provider{detector: defaultDetector()}
}

func (p *Provider) ID() string   { return "emulation" }
func (p *Provider) Name() string { return "Emuladores" }

func (p *Provider) IsLocal() bool                                       { return true }
func (p *Provider) Authenticate(context.Context, store.StoreAuth) error { return nil }
func (p *Provider) IsAuthenticated() bool                               { return true }

// SetFolders substitui as pastas extras de ROMs. As pastas padrão do EmuDeck
// continuam sendo verificadas.
func (p *Provider) SetFolders(folders []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.folders = cleanFolders(folders)
}

func (p *Provider) Folders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.folders...)
}

// DefaultRoots devolve os locais usuais de ROMs no Linux.
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "ROMs"),
		filepath.Join(home, "Emulation/roms"),
		filepath.Join(home, "Emulation"),
		filepath.Join(home, ".local/share/Emulation/roms"),
	}
}

func (p *Provider) FetchLibrary(context.Context) ([]*models.Game, error) {
	p.mu.RLock()
	extra := append([]string(nil), p.folders...)
	p.mu.RUnlock()
	roots := append(DefaultRoots(), extra...)
	available := p.detector.Detect()
	return scanROMs(roots, available), nil
}

func (p *Provider) LaunchCommand(game *models.Game) (string, []string, error) {
	if game == nil {
		return "", nil, fmt.Errorf("emulação: jogo inválido")
	}
	if strings.TrimSpace(game.Executable) == "" {
		return "", nil, fmt.Errorf("emulação: nenhum emulador disponível para %s", game.Name)
	}
	return game.Executable, append([]string(nil), game.Arguments...), nil
}

func (p *Provider) UninstallCommand(*models.Game) (string, []string, error) {
	return "", nil, fmt.Errorf("emulação: remova a ROM manualmente da pasta")
}

// scanROMs percorre as raízes e classifica arquivos pela pasta da plataforma,
// como ~/ROMs/ps2/jogo.iso ou ~/Emulation/roms/switch/jogo.nsp.
func scanROMs(roots []string, available map[string]Emulator) []*models.Game {
	seen := make(map[string]bool)
	var games []*models.Game

	for _, root := range cleanFolders(roots) {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if path != root && strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || seen[path] {
				return nil
			}
			system := systemForFile(root, path)
			if system == nil {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if !contains(system.Extensions, ext) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}

			emulator := pickEmulator(system, available)
			game := &models.Game{
				GameID:           "rom:" + path,
				PluginID:         "emulation",
				Source:           "Emulação",
				Name:             romName(path),
				SortingName:      romName(path),
				IsInstalled:      true,
				InstallDirectory: filepath.Dir(path),
				InstallSize:      uint64(info.Size()),
				Platforms:        []string{system.Name},
				Tags:             []string{"ROM", system.Name},
			}
			if emulator != nil {
				game.Executable, game.Arguments = emulator.Command(path)
				game.Tags = append(game.Tags, emulator.Name)
			}
			seen[path] = true
			games = append(games, game)
			return nil
		})
	}

	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})
	return games
}

func systemForFile(root, path string) *System {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	// A pasta imediatamente acima do arquivo é a plataforma. Também aceitamos
	// a própria raiz quando o usuário aponta direto para ~/ROMs/ps2.
	for index := len(parts) - 2; index >= 0; index-- {
		if system := systemByAlias(parts[index]); system != nil {
			return system
		}
	}
	if len(parts) == 1 {
		return systemByAlias(filepath.Base(root))
	}
	return nil
}

func systemByAlias(value string) *System {
	normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(value))
	key := aliases[normalized]
	if key == "" {
		return nil
	}
	for index := range systems {
		if systems[index].Key == key {
			return &systems[index]
		}
	}
	return nil
}

func pickEmulator(system *System, available map[string]Emulator) *Emulator {
	for _, id := range system.Emulators {
		if emulator, ok := available[id]; ok {
			copy := emulator
			return &copy
		}
	}
	return nil
}

func romName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.NewReplacer("_", " ", ".", " ").Replace(base)
	return strings.Join(strings.Fields(base), " ")
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func cleanFolders(folders []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(folders))
	for _, folder := range folders {
		folder = filepath.Clean(strings.TrimSpace(folder))
		if folder == "" || folder == "." || seen[folder] {
			continue
		}
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			continue
		}
		seen[folder] = true
		result = append(result, folder)
	}
	sort.Strings(result)
	return result
}
