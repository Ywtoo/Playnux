package wine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/store/builtins/heroic"
	"github.com/playnux/core/store/builtins/steam"
)

// Runner é uma versão de Proton/Wine encontrada no sistema.
type Runner struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Version     string `json:"version,omitempty"`
	Recommended bool   `json:"recommended"`
}

// DiscoverRunners lista Proton (Steam/GE) e o Wine do sistema.
func DiscoverRunners() []Runner {
	seen := make(map[string]bool)
	runners := make([]Runner, 0, 8)
	addProton := func(path string) {
		path = filepath.Clean(path)
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		if seen[path] {
			return
		}
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return
		}
		seen[path] = true
		runners = append(runners, Runner{
			Name:    filepath.Base(filepath.Dir(path)),
			Path:    path,
			Kind:    "proton",
			Version: protonBuild(path),
		})
	}

	var bases []string
	for _, root := range steam.Roots() {
		bases = append(bases, filepath.Join(root, "compatibilitytools.d"))
	}
	for _, library := range steam.LibraryDirectories() {
		bases = append(bases, filepath.Join(library, "common"))
	}
	if config, err := os.UserConfigDir(); err == nil {
		bases = append(bases, filepath.Join(config, "heroic/tools/proton"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(home, ".var/app/com.heroicgameslauncher.hgl/config/heroic/tools/proton"))
	}
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.Contains(strings.ToLower(entry.Name()), "proton") {
				addProton(filepath.Join(base, entry.Name(), "proton"))
			}
		}
	}

	if wine, lookupErr := exec.LookPath("wine"); lookupErr == nil {
		runners = append(runners, Runner{
			Name: "Wine do sistema",
			Path: wine,
			Kind: "wine",
		})
	}

	sort.SliceStable(runners, func(i, j int) bool {
		left, right := runners[i], runners[j]
		if left.Kind != right.Kind {
			return left.Kind == "proton"
		}
		leftVersion, rightVersion := stableVersion(left.Name), stableVersion(right.Name)
		if leftVersion != rightVersion {
			return leftVersion > rightVersion
		}
		return strings.ToLower(left.Name) > strings.ToLower(right.Name)
	})
	return runners
}

func protonBuild(executable string) string {
	path := filepath.Join(filepath.Dir(executable), "version")
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(fields[1], "-x86_64"), "-arm64")
}

var stableProton = regexp.MustCompile(`(?i)^Proton (\d+)\.(\d+)$`)

func stableVersion(name string) int {
	match := stableProton.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	return 1 + major*100 + minor
}

// ManualWindowsExecutable returns the .exe added manually, when present.
func ManualWindowsExecutable(game *models.Game) string {
	if game == nil || game.PluginID != "linux" {
		return ""
	}
	path := strings.TrimPrefix(game.GameID, "manual:")
	if path == game.GameID || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}

// WindowsExecutable resolves the Windows .exe behind a manual, Steam, GOG or
// Epic game. GOG/Epic use Heroic/Legendary metadata; Steam and manual games
// fall back to a shallow scan of the install directory.
func WindowsExecutable(game *models.Game) string {
	if game == nil {
		return ""
	}
	if exe := ManualWindowsExecutable(game); exe != "" {
		return exe
	}
	if strings.EqualFold(filepath.Ext(game.Executable), ".exe") {
		if info, err := os.Stat(game.Executable); err == nil && !info.IsDir() {
			return game.Executable
		}
	}
	if game.PluginID == "gog" || game.PluginID == "epic" {
		if install := heroic.InstallFor(game.GameID); install != nil {
			if strings.EqualFold(install.Platform, "linux") {
				return ""
			}
			if info, err := os.Stat(install.Path); err == nil && info.IsDir() {
				if exe := install.ExecutablePath(); exe != "" && strings.EqualFold(filepath.Ext(exe), ".exe") {
					if info, err := os.Stat(exe); err == nil && !info.IsDir() {
						return exe
					}
				}
				if exe := findExecutable(install.Path, game.Name); exe != "" {
					return exe
				}
			}
		}
	}
	if game.InstallDirectory != "" {
		if info, err := os.Stat(game.InstallDirectory); err == nil && info.IsDir() {
			return findExecutable(game.InstallDirectory, game.Name)
		}
	}
	return ""
}

var ignoredExecutableWords = []string{
	"unins", "setup", "install", "redist", "vcredist", "dxsetup", "dxwebsetup",
	"crash", "report", "launcher_installer", "eula", "activation",
}

func findExecutable(root, name string) string {
	candidates := make(map[string]string)
	title := executableKey(name)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel != "." {
				depth := len(strings.Split(filepath.ToSlash(rel), "/"))
				if depth >= 3 {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		lower := strings.ToLower(filepath.Base(path))
		for _, word := range ignoredExecutableWords {
			if strings.Contains(lower, word) {
				return nil
			}
		}
		key := executableKey(strings.TrimSuffix(lower, ".exe"))
		current := candidates[key]
		if current == "" || (is64BitPath(path) && !is64BitPath(current)) {
			candidates[key] = path
		}
		return nil
	})
	if candidate := candidates[title]; candidate != "" {
		return candidate
	}
	if len(candidates) == 1 {
		for _, candidate := range candidates {
			return candidate
		}
	}
	return ""
}

func executableKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(name))
}

func is64BitPath(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		switch strings.ToLower(segment) {
		case "x64", "win64", "64bit", "x86_64":
			return true
		}
	}
	return false
}

// Apply builds an isolated Proton/Wine invocation. Native store launchers stay
// the default; a custom runner is only used when the user explicitly selects
// it (manual games also honour the automatic Proton mode).
func Apply(options *launcher.Options, game *models.Game, dataDir string) error {
	if game == nil || game.PluginID == "emulation" {
		return nil
	}
	runner := strings.TrimSpace(options.Runner)
	protonPath := strings.TrimSpace(options.ProtonPath)
	if runner == "" {
		runner = "auto"
	}

	if runner == "auto" && game.PluginID != "linux" {
		return nil
	}
	exe := WindowsExecutable(game)
	if runner == "wine" {
		if exe == "" {
			return fmt.Errorf("não foi possível localizar o executável Windows deste jogo")
		}
		wine, err := exec.LookPath("wine")
		if err != nil {
			return fmt.Errorf("o Wine do sistema não foi encontrado")
		}
		options.Command = wine
		options.CommandArgs = []string{exe}
		if options.Directory == "" {
			options.Directory = filepath.Dir(exe)
		}
		return nil
	}

	if runner == "auto" {
		if game.PluginID != "linux" || exe == "" {
			return nil
		}
		for _, candidate := range DiscoverRunners() {
			if candidate.Kind == "proton" {
				protonPath = candidate.Path
				break
			}
		}
		if protonPath == "" {
			return nil
		}
		runner = "proton"
	}
	if runner != "proton" {
		return fmt.Errorf("runner de compatibilidade desconhecido")
	}
	if exe == "" {
		return fmt.Errorf("não foi possível localizar o executável Windows deste jogo")
	}
	info, err := os.Stat(protonPath)
	if err != nil || info.IsDir() {
		return fmt.Errorf("o Proton selecionado não está mais disponível")
	}

	prefix := filepath.Join(dataDir, "compatdata", prefixID(game.ID))
	if err := os.MkdirAll(prefix, 0o700); err != nil {
		return fmt.Errorf("criar prefixo Proton: %w", err)
	}
	options.Command = protonPath
	options.CommandArgs = []string{"run", exe}
	options.Environment = map[string]string{
		"STEAM_COMPAT_DATA_PATH":           prefix,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH": steamInstallRoot(protonPath),
	}
	if options.Directory == "" {
		options.Directory = filepath.Dir(exe)
	}
	return nil
}

func prefixID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func steamInstallRoot(protonPath string) string {
	roots := steam.Roots()
	for _, root := range roots {
		if strings.HasPrefix(protonPath, root+string(filepath.Separator)) {
			return root
		}
	}
	if len(roots) > 0 {
		return roots[0]
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".steam/root")
}

// RunnerName devolve o nome legível de uma versão de Proton/Wine pelo caminho.
func RunnerName(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(filepath.Dir(path))
}
