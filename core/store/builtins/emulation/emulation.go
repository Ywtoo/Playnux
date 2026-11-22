// Package emulation detects installed emulators and builds a library from ROM
// folders. Profiles describe the launch command template, supported platforms
// and extensions, following the Playnite EmulatorProfile model.
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

// Emulator describes an installed emulator, either native or Flatpak.
type Emulator struct {
	ID        string
	Name      string
	Binary    string
	FlatpakID string
}

// Command returns the executable and arguments used to open a ROM.
func (e Emulator) Command(rom string) (string, []string) {
	if e.Binary != "" {
		return e.Binary, []string{rom}
	}
	if e.FlatpakID != "" {
		return "flatpak", []string{"run", e.FlatpakID, rom}
	}
	return "", nil
}

// EmulatorProfile is a launch recipe for one emulator and platform family.
type EmulatorProfile struct {
	ProfileID    string   `json:"profile_id"`
	EmulatorID   string   `json:"emulator_id"`
	Name         string   `json:"name"`
	Platforms    []string `json:"platforms"`
	Extensions   []string `json:"extensions"`
	Executable   string   `json:"executable,omitempty"`
	FlatpakID    string   `json:"flatpak_id,omitempty"`
	ArgsTemplate string   `json:"args_template"`
	// FolderAliases are normalized folder names mapped to this profile, such
	// as "ps2" or "playstation2". They power automatic ROM folder detection.
	FolderAliases []string `json:"folder_aliases,omitempty"`
	// Priority decides which profile wins when several support the same file.
	Priority int  `json:"priority,omitempty"`
	Builtin  bool `json:"builtin,omitempty"`
}

// Command expands the argument template for a ROM path.
func (p EmulatorProfile) Command(rom string) (string, []string, error) {
	if p.Executable == "" && p.FlatpakID == "" {
		return "", nil, fmt.Errorf("emulation: no executable configured for %s", p.Name)
	}
	args := expandTemplate(p.ArgsTemplate, rom)
	if p.Executable != "" {
		return p.Executable, args, nil
	}
	return "flatpak", append([]string{"run", p.FlatpakID}, args...), nil
}

// Supports reports whether the profile accepts the given file extension.
func (p EmulatorProfile) Supports(path string) bool {
	return contains(p.Extensions, strings.ToLower(filepath.Ext(path)))
}

// EmulatorDefinition groups an emulator with its profiles.
type EmulatorDefinition struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Executable string            `json:"executable,omitempty"`
	FlatpakID  string            `json:"flatpak_id,omitempty"`
	Profiles   []EmulatorProfile `json:"profiles"`
	Custom     bool              `json:"custom,omitempty"`
}

// Detected reports whether the emulator executable was found on this system.
func (d EmulatorDefinition) Detected() bool {
	return d.Executable != "" || d.FlatpakID != ""
}

// CustomEmulator is the user-editable payload persisted in SQLite.
type CustomEmulator struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Executable string            `json:"executable"`
	FlatpakID  string            `json:"flatpak_id"`
	Profiles   []EmulatorProfile `json:"profiles"`
}

// System associates a ROM folder family with preferred emulators.
type System struct {
	Key        string
	Name       string
	Extensions []string
	Emulators  []string
}

var systems = []System{
	{Key: "ps2", Name: "PlayStation 2", Extensions: []string{".iso", ".chd", ".bin", ".cue", ".zso"}, Emulators: []string{"pcsx2", "retroarch"}},
	{Key: "ps3", Name: "PlayStation 3", Extensions: []string{".iso", ".pkg", ".dec"}, Emulators: []string{"rpcs3"}},
	{Key: "switch", Name: "Nintendo Switch", Extensions: []string{".nsp", ".xci", ".nro"}, Emulators: []string{"ryujinx"}},
	{Key: "gamecube", Name: "Nintendo GameCube", Extensions: []string{".iso", ".gcm", ".rvz"}, Emulators: []string{"dolphin", "retroarch"}},
	{Key: "wii", Name: "Nintendo Wii", Extensions: []string{".iso", ".wbfs", ".rvz"}, Emulators: []string{"dolphin", "retroarch"}},
	{Key: "wiiu", Name: "Nintendo Wii U", Extensions: []string{".wud", ".wux", ".rpx"}, Emulators: []string{"cemu"}},
	{Key: "psp", Name: "PlayStation Portable", Extensions: []string{".iso", ".cso", ".chd"}, Emulators: []string{"ppsspp"}},
	{Key: "ps1", Name: "PlayStation", Extensions: []string{".bin", ".cue", ".chd", ".iso", ".pbp", ".m3u"}, Emulators: []string{"duckstation", "retroarch"}},
	{Key: "snes", Name: "Super Nintendo", Extensions: []string{".sfc", ".smc", ".fig", ".swc"}, Emulators: []string{"retroarch"}},
	{Key: "nes", Name: "Nintendo Entertainment System", Extensions: []string{".nes", ".fds", ".unf"}, Emulators: []string{"retroarch"}},
	{Key: "gba", Name: "Game Boy Advance", Extensions: []string{".gba"}, Emulators: []string{"retroarch"}},
	{Key: "gb", Name: "Game Boy", Extensions: []string{".gb", ".gbc"}, Emulators: []string{"retroarch"}},
	{Key: "n64", Name: "Nintendo 64", Extensions: []string{".n64", ".z64", ".v64"}, Emulators: []string{"retroarch"}},
	{Key: "nds", Name: "Nintendo DS", Extensions: []string{".nds"}, Emulators: []string{"melonds", "retroarch"}},
	{Key: "genesis", Name: "Mega Drive / Genesis", Extensions: []string{".md", ".gen", ".smd"}, Emulators: []string{"retroarch"}},
	{Key: "dreamcast", Name: "Dreamcast", Extensions: []string{".gdi", ".cdi", ".chd"}, Emulators: []string{"retroarch"}},
	{Key: "saturn", Name: "Sega Saturn", Extensions: []string{".chd", ".cue", ".bin"}, Emulators: []string{"retroarch"}},
	{Key: "arcade", Name: "Arcade", Extensions: []string{".zip", ".7z"}, Emulators: []string{"retroarch"}},
}

var aliases = map[string]string{
	"ps2": "ps2", "playstation2": "ps2",
	"ps3": "ps3", "playstation3": "ps3",
	"switch": "switch", "yuzu": "switch", "ryujinx": "switch",
	"gamecube": "gamecube", "gc": "gamecube", "ngc": "gamecube",
	"wii":  "wii",
	"wiiu": "wiiu", "cemu": "wiiu",
	"psp": "psp", "playstationportable": "psp",
	"ps1": "ps1", "psx": "ps1", "playstation": "ps1",
	"snes": "snes", "supernintendo": "snes",
	"nes": "nes", "famicom": "nes",
	"gba": "gba", "gameboyadvance": "gba",
	"gb": "gb", "gbc": "gb", "gameboy": "gb",
	"n64": "n64", "nintendo64": "n64",
	"nds": "nds", "ds": "nds", "melonds": "nds",
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
	{Emulator: Emulator{ID: "cemu", Name: "Cemu"}, Binaries: []string{"cemu", "Cemu"}, FlatpakIDs: []string{"info.cemu.Cemu"}},
	{Emulator: Emulator{ID: "melonds", Name: "melonDS"}, Binaries: []string{"melonds", "melonDS"}, FlatpakIDs: []string{"net.kuribo64.melonDS"}},
}

// Argument templates used by detected emulators. {ImagePath} and %ROM% are
// replaced with the absolute ROM path; if neither is present the ROM is
// appended at the end.
var argsTemplates = map[string]string{
	"retroarch":   `-f "{ImagePath}"`,
	"pcsx2":       `--fullscreen --batch "{ImagePath}"`,
	"rpcs3":       `"{ImagePath}"`,
	"dolphin":     `-b -e "{ImagePath}"`,
	"duckstation": `-fullscreen "{ImagePath}"`,
	"ryujinx":     `"{ImagePath}"`,
	"ppsspp":      `--fullscreen "{ImagePath}"`,
	"cemu":        `-f -g "{ImagePath}"`,
	"melonds":     `"{ImagePath}"`,
}

// IsBuiltinEmulator reports whether an emulator ID belongs to the catalog
// shipped with Playnux.
func IsBuiltinEmulator(id string) bool {
	id = normalizeID(id)
	for _, def := range emulatorDefs {
		if def.ID == id {
			return true
		}
	}
	return false
}

// IsEmulatorApplication identifies installed emulator applications, separately
// from ROMs, so they are not imported as regular desktop games.
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

// Detector can be replaced in tests to avoid depending on the host.
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

// Detect returns available emulators indexed by logical ID.
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

// Catalog combines detected built-in emulators with user-defined ones.
// Built-in emulators are ONLY included if they are actually detected on the host
// system, ensuring no phantom emulators or uninstalled software are displayed.
func Catalog(available map[string]Emulator, custom []EmulatorDefinition) []EmulatorDefinition {
	definitions := make([]EmulatorDefinition, 0, len(available)+len(custom))
	for _, system := range systems {
		for rank, emulatorID := range system.Emulators {
			// If the emulator is not installed/detected, do not create or show it!
			if _, ok := available[emulatorID]; !ok {
				continue
			}
			def := definitionFor(emulatorID, available)
			if def == nil || !def.Detected() || hasProfileFor(def.Profiles, system.Key) {
				continue
			}
			profile := profileFor(def, system, rank)
			for index := range definitions {
				if definitions[index].ID == def.ID {
					definitions[index].Profiles = append(definitions[index].Profiles, profile)
					def = nil
					break
				}
			}
			if def == nil {
				continue
			}
			def.Profiles = []EmulatorProfile{profile}
			definitions = append(definitions, *def)
		}
	}
	// Emulators detected on the system without an auto-mapped system are listed
	// so the user can bind folders or custom profiles to them.
	for _, def := range emulatorDefs {
		emulator, isAvailable := available[def.ID]
		if !isAvailable || (emulator.Binary == "" && emulator.FlatpakID == "") {
			continue
		}
		found := false
		for _, existing := range definitions {
			if existing.ID == def.ID {
				found = true
				break
			}
		}
		if found {
			continue
		}
		definitions = append(definitions, EmulatorDefinition{
			ID:         emulator.ID,
			Name:       emulator.Name,
			Executable: emulator.Binary,
			FlatpakID:  emulator.FlatpakID,
		})
	}
	for _, customDef := range custom {
		normalized, err := normalizeCustom(customDef)
		if err != nil {
			continue
		}
		definitions = append(definitions, normalized)
	}
	sort.SliceStable(definitions, func(i, j int) bool {
		if definitions[i].Detected() != definitions[j].Detected() {
			return definitions[i].Detected()
		}
		return strings.ToLower(definitions[i].Name) < strings.ToLower(definitions[j].Name)
	})
	return definitions
}

func definitionFor(id string, available map[string]Emulator) *EmulatorDefinition {
	for _, def := range emulatorDefs {
		if def.ID != id {
			continue
		}
		emulator := available[id]
		if emulator.ID == "" {
			emulator = def.Emulator
		}
		return &EmulatorDefinition{
			ID:         emulator.ID,
			Name:       emulator.Name,
			Executable: emulator.Binary,
			FlatpakID:  emulator.FlatpakID,
		}
	}
	return nil
}

func profileFor(def *EmulatorDefinition, system System, rank int) EmulatorProfile {
	return EmulatorProfile{
		ProfileID:     fmt.Sprintf("%s:%s", def.ID, system.Key),
		EmulatorID:    def.ID,
		Name:          fmt.Sprintf("%s - %s", def.Name, system.Name),
		Platforms:     []string{system.Name},
		Extensions:    append([]string(nil), system.Extensions...),
		Executable:    def.Executable,
		FlatpakID:     def.FlatpakID,
		ArgsTemplate:  argsTemplates[def.ID],
		FolderAliases: folderAliases(system.Key),
		Priority:      rank,
		Builtin:       true,
	}
}

func hasProfileFor(profiles []EmulatorProfile, key string) bool {
	suffix := ":" + key
	for _, profile := range profiles {
		if strings.HasSuffix(profile.ProfileID, suffix) {
			return true
		}
	}
	return false
}

func folderAliases(key string) []string {
	result := []string{key}
	for alias, target := range aliases {
		if target == key && alias != key {
			result = append(result, alias)
		}
	}
	sort.Strings(result)
	return result
}

// NormalizeCustom validates a user-defined emulator before it is persisted.
func NormalizeCustom(def EmulatorDefinition) (EmulatorDefinition, error) {
	return normalizeCustom(def)
}

func normalizeCustom(def EmulatorDefinition) (EmulatorDefinition, error) {
	def.ID = normalizeID(def.ID)
	if def.ID == "" {
		return EmulatorDefinition{}, fmt.Errorf("emulation: emulator id is required")
	}
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		def.Name = def.ID
	}
	def.Executable = strings.TrimSpace(def.Executable)
	def.FlatpakID = strings.TrimSpace(def.FlatpakID)
	if def.Executable == "" && def.FlatpakID == "" {
		return EmulatorDefinition{}, fmt.Errorf("emulation: inform an executable or a Flatpak ID")
	}
	if len(def.Profiles) == 0 {
		return EmulatorDefinition{}, fmt.Errorf("emulation: at least one profile is required")
	}
	seen := make(map[string]bool)
	profiles := make([]EmulatorProfile, 0, len(def.Profiles))
	for index, profile := range def.Profiles {
		profile.EmulatorID = def.ID
		profile.ProfileID = strings.TrimSpace(profile.ProfileID)
		if profile.ProfileID == "" {
			profile.ProfileID = fmt.Sprintf("%s:%d", def.ID, index+1)
		}
		if seen[profile.ProfileID] {
			return EmulatorDefinition{}, fmt.Errorf("emulation: duplicate profile id %q", profile.ProfileID)
		}
		seen[profile.ProfileID] = true
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.Name == "" {
			profile.Name = def.Name
		}
		if profile.Executable == "" {
			profile.Executable = def.Executable
		}
		if profile.FlatpakID == "" {
			profile.FlatpakID = def.FlatpakID
		}
		profile.Extensions = normalizeExtensions(profile.Extensions)
		if len(profile.Extensions) == 0 {
			return EmulatorDefinition{}, fmt.Errorf("emulation: profile %q needs at least one extension", profile.Name)
		}
		profile.Platforms = normalizeList(profile.Platforms)
		if profile.ArgsTemplate == "" {
			profile.ArgsTemplate = `"{ImagePath}"`
		}
		if len(profile.FolderAliases) == 0 {
			for _, platform := range profile.Platforms {
				profile.FolderAliases = append(profile.FolderAliases, normalizeAlias(platform))
			}
		}
		profile.FolderAliases = normalizeList(profile.FolderAliases)
		profile.Priority = 100 + index
		profile.Builtin = false
		profiles = append(profiles, profile)
	}
	def.Profiles = profiles
	def.Custom = true
	return def, nil
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-' || char == '_' || char == '.':
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func normalizeAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
}

func normalizeExtensions(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		if !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

// FindProfile returns a profile from the catalog by its ID.
func FindProfile(catalog []EmulatorDefinition, profileID string) (EmulatorProfile, bool) {
	for _, definition := range catalog {
		for _, profile := range definition.Profiles {
			if profile.ProfileID == profileID {
				return profile, true
			}
		}
	}
	return EmulatorProfile{}, false
}

// Provider is the local source of emulation games.
type Provider struct {
	mu             sync.RWMutex
	folders        []string
	folderProfiles map[string]string
	custom         []EmulatorDefinition
	catalog        []EmulatorDefinition
	catalogReady   bool
	detector       Detector
}

func NewProvider() *Provider {
	return &Provider{
		detector:       defaultDetector(),
		folderProfiles: make(map[string]string),
	}
}

func (p *Provider) ID() string   { return "emulation" }
func (p *Provider) Name() string { return "Emuladores" }

func (p *Provider) IsLocal() bool                                       { return true }
func (p *Provider) Authenticate(context.Context, store.StoreAuth) error { return nil }
func (p *Provider) IsAuthenticated() bool                               { return true }

// SetFolders replaces extra ROM folders. Standard EmuDeck folders are always
// checked, and mapped folders use their chosen emulator profile.
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

// SetFolderProfiles maps extra ROM folders to a specific emulator profile.
func (p *Provider) SetFolderProfiles(mapping map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.folderProfiles = make(map[string]string, len(mapping))
	for folder, profileID := range mapping {
		if folder = filepath.Clean(strings.TrimSpace(folder)); folder != "" {
			p.folderProfiles[folder] = profileID
		}
	}
}

func (p *Provider) FolderProfiles() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make(map[string]string, len(p.folderProfiles))
	for folder, profileID := range p.folderProfiles {
		result[folder] = profileID
	}
	return result
}

// SetCustom replaces the user-defined emulators and refreshes the catalog.
func (p *Provider) SetCustom(definitions []EmulatorDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.custom = normalizeCustomList(definitions)
	p.catalogReady = false
}

func normalizeCustomList(definitions []EmulatorDefinition) []EmulatorDefinition {
	result := make([]EmulatorDefinition, 0, len(definitions))
	for _, definition := range definitions {
		normalized, err := normalizeCustom(definition)
		if err == nil {
			result = append(result, normalized)
		}
	}
	return result
}

// Catalog returns detected and custom emulators with their profiles.
func (p *Provider) Catalog() []EmulatorDefinition {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCatalogLocked()
	return cloneCatalog(p.catalog)
}

func (p *Provider) ensureCatalogLocked() {
	if p.catalogReady {
		return
	}
	p.catalog = Catalog(p.detector.Detect(), p.custom)
	p.catalogReady = true
}

func cloneCatalog(catalog []EmulatorDefinition) []EmulatorDefinition {
	result := make([]EmulatorDefinition, len(catalog))
	for index, definition := range catalog {
		result[index] = definition
		result[index].Profiles = append([]EmulatorProfile(nil), definition.Profiles...)
	}
	return result
}

// DefaultRoots returns the usual Linux ROM locations.
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
	p.mu.Lock()
	extra := append([]string(nil), p.folders...)
	folderProfiles := make(map[string]string, len(p.folderProfiles))
	for folder, profileID := range p.folderProfiles {
		folderProfiles[folder] = profileID
	}
	p.ensureCatalogLocked()
	catalog := cloneCatalog(p.catalog)
	p.mu.Unlock()

	roots := append(DefaultRoots(), extra...)
	return scanCatalog(roots, folderProfiles, catalog), nil
}

func (p *Provider) LaunchCommand(game *models.Game) (string, []string, error) {
	if game == nil {
		return "", nil, fmt.Errorf("emulation: invalid game")
	}
	if strings.TrimSpace(game.Executable) == "" {
		return "", nil, fmt.Errorf("emulation: no emulator available for %s", game.Name)
	}
	return game.Executable, append([]string(nil), game.Arguments...), nil
}

func (p *Provider) UninstallCommand(*models.Game) (string, []string, error) {
	return "", nil, fmt.Errorf("emulation: remove the ROM manually from its folder")
}

// scanCatalog walks the roots and assigns each ROM to the best profile.
// Folders with an explicit profile are scanned only with that profile.
func scanCatalog(roots []string, folderProfiles map[string]string, catalog []EmulatorDefinition) []*models.Game {
	seen := make(map[string]bool)
	var games []*models.Game
	profiles := usableProfiles(catalog)

	for _, root := range cleanFolders(roots) {
		if profileID := folderProfiles[root]; profileID != "" {
			profile, found := FindProfile(catalog, profileID)
			if !found || (profile.Executable == "" && profile.FlatpakID == "") {
				continue
			}
			games = append(games, scanWithProfile(root, profile, seen)...)
			continue
		}
		games = append(games, scanAutomatic(root, profiles, seen)...)
	}

	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})
	return games
}

func usableProfiles(catalog []EmulatorDefinition) []EmulatorProfile {
	var result []EmulatorProfile
	for _, definition := range catalog {
		for _, profile := range definition.Profiles {
			if profile.Executable == "" && profile.FlatpakID == "" {
				continue
			}
			result = append(result, profile)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Priority < result[j].Priority })
	return result
}

func scanAutomatic(root string, profiles []EmulatorProfile, seen map[string]bool) []*models.Game {
	var games []*models.Game
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if skipDirectory(root, path, entry) {
			return filepath.SkipDir
		}
		game, ok := scanFile(path, entry, walkErr, func(path string) (EmulatorProfile, bool) {
			return bestProfile(path, profiles)
		}, seen)
		if ok {
			games = append(games, game)
		}
		return nil
	})
	return games
}

func scanWithProfile(root string, profile EmulatorProfile, seen map[string]bool) []*models.Game {
	var games []*models.Game
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if skipDirectory(root, path, entry) {
			return filepath.SkipDir
		}
		game, ok := scanFile(path, entry, walkErr, func(path string) (EmulatorProfile, bool) {
			if profile.Supports(path) {
				return profile, true
			}
			return EmulatorProfile{}, false
		}, seen)
		if ok {
			games = append(games, game)
		}
		return nil
	})
	return games
}

func skipDirectory(root, path string, entry os.DirEntry) bool {
	return entry.IsDir() && path != root && strings.HasPrefix(entry.Name(), ".")
}

func scanFile(path string, entry os.DirEntry, walkErr error, pick func(string) (EmulatorProfile, bool), seen map[string]bool) (*models.Game, bool) {
	if walkErr != nil || entry.IsDir() {
		return nil, false
	}
	if entry.Type()&os.ModeSymlink != 0 || seen[path] {
		return nil, false
	}
	profile, ok := pick(path)
	if !ok {
		return nil, false
	}
	info, err := entry.Info()
	if err != nil {
		return nil, false
	}
	executable, args, err := profile.Command(path)
	if err != nil {
		return nil, false
	}
	platform := "Emulação"
	platforms := profile.Platforms
	if len(platforms) > 0 {
		platform = platforms[0]
	}
	seen[path] = true
	game := &models.Game{
		GameID:           "rom:" + path,
		PluginID:         "emulation",
		Source:           "Emulação",
		Name:             romName(path),
		SortingName:      romName(path),
		IsInstalled:      true,
		InstallDirectory: filepath.Dir(path),
		InstallSize:      uint64(info.Size()),
		Platforms:        append([]string(nil), platforms...),
		Tags:             []string{"ROM", platform},
		Executable:       executable,
		Arguments:        args,
	}
	if profile.Name != "" {
		game.Tags = append(game.Tags, profile.Name)
	}
	return game, true
}

func bestProfile(path string, profiles []EmulatorProfile) (EmulatorProfile, bool) {
	components := pathComponents(path)
	var chosen EmulatorProfile
	found := false
	for _, profile := range profiles {
		if !profile.Supports(path) {
			continue
		}
		if !folderMatches(profile.FolderAliases, components) {
			continue
		}
		if !found || profile.Priority < chosen.Priority {
			chosen = profile
			found = true
		}
	}
	return chosen, found
}

func pathComponents(path string) map[string]bool {
	parts := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	result := make(map[string]bool, len(parts))
	for _, part := range parts {
		result[normalizeAlias(part)] = true
	}
	return result
}

func folderMatches(folderAliases []string, components map[string]bool) bool {
	for _, alias := range folderAliases {
		if components[normalizeAlias(alias)] {
			return true
		}
	}
	return false
}

func expandTemplate(template, rom string) []string {
	if strings.TrimSpace(template) == "" {
		return []string{rom}
	}
	fields := splitArgs(template)
	args := make([]string, 0, len(fields)+1)
	replaced := false
	for _, field := range fields {
		if strings.Contains(field, "{ImagePath}") || strings.Contains(field, "%ROM%") {
			replaced = true
			field = strings.ReplaceAll(field, "{ImagePath}", rom)
			field = strings.ReplaceAll(field, "%ROM%", rom)
		}
		args = append(args, field)
	}
	if !replaced {
		args = append(args, rom)
	}
	return args
}

// splitArgs is a small shell-like tokenizer that honours single and double
// quotes. It does not execute a shell, so templates cannot expand variables.
func splitArgs(value string) []string {
	var result []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
	}
	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		switch {
		case char == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
		case char == '\'' || char == '"':
			quote = char
		case char == ' ' || char == '\t' || char == '\n':
			flush()
		default:
			current.WriteRune(char)
		}
	}
	flush()
	return result
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
