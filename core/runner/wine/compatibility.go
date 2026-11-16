package wine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/playnux/core/models"
	"github.com/playnux/core/runner/launcher"
	"github.com/playnux/core/store/builtins/heroic"
)

// Runner é uma versão de Proton/Wine encontrada no sistema.
type Runner struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Recommended bool   `json:"recommended"`
}

// DiscoverRunners lista Proton (Steam/GE) e o Wine do sistema.
func DiscoverRunners() []Runner {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var roots []string
	for _, root := range []string{
		filepath.Join(home, ".local/share/Steam"),
		filepath.Join(home, ".steam/steam"),
		filepath.Join(home, ".steam/root"),
		filepath.Join(home, ".var/app/com.valvesoftware.Steam/.local/share/Steam"),
		filepath.Join(home, ".var/app/com.valvesoftware.Steam/data/Steam"),
	} {
		if info, statErr := os.Stat(root); statErr == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}

	seen := make(map[string]bool)
	runners := make([]Runner, 0, 8)
	addProton := func(path string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		info, statErr := os.Stat(path)
		if statErr != nil || info.IsDir() {
			return
		}
		seen[path] = true
		runners = append(runners, Runner{
			Name: filepath.Base(filepath.Dir(path)),
			Path: path,
			Kind: "proton",
		})
	}

	for _, root := range roots {
		for _, base := range []string{
			filepath.Join(root, "compatibilitytools.d"),
			filepath.Join(root, "steamapps", "common"),
		} {
			entries, readErr := os.ReadDir(base)
			if readErr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() || !strings.Contains(strings.ToLower(entry.Name()), "proton") {
					continue
				}
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
		leftGE := strings.HasPrefix(strings.ToLower(left.Name), "ge-proton")
		rightGE := strings.HasPrefix(strings.ToLower(right.Name), "ge-proton")
		if leftGE != rightGE {
			return leftGE
		}
		return strings.ToLower(left.Name) > strings.ToLower(right.Name)
	})
	if len(runners) > 0 && runners[0].Kind == "proton" {
		runners[0].Recommended = true
	}
	return runners
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
			if info, err := os.Stat(install.Path); err == nil && info.IsDir() {
				if exe := install.ExecutablePath(); exe != "" && strings.EqualFold(filepath.Ext(exe), ".exe") {
					if info, err := os.Stat(exe); err == nil && !info.IsDir() {
						return exe
					}
				}
				if exe := findExecutable(install.Path); exe != "" {
					return exe
				}
			}
		}
	}
	if game.InstallDirectory != "" {
		if info, err := os.Stat(game.InstallDirectory); err == nil && info.IsDir() {
			return findExecutable(game.InstallDirectory)
		}
	}
	return ""
}

var ignoredExecutableWords = []string{
	"unins", "setup", "install", "redist", "vcredist", "dxsetup", "dxwebsetup",
	"crash", "report", "launcher_installer", "eula", "activation",
}

func findExecutable(root string) string {
	var best string
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
		if best == "" || len(path) < len(best) {
			best = path
		}
		return nil
	})
	return best
}

// Apply builds an isolated Proton/Wine invocation. Native store launchers stay
// the default; a custom runner is only used when the user explicitly selects
// it (manual games also honour the automatic Proton mode).
func Apply(options *launcher.Options, game *models.Game, dataDir string) error {
	exe := WindowsExecutable(game)
	runner := strings.TrimSpace(options.Runner)
	protonPath := strings.TrimSpace(options.ProtonPath)
	if runner == "" {
		runner = "auto"
	}

	if runner == "wine" {
		if exe == "" {
			return fmt.Errorf("não foi possível localizar o executável Windows deste jogo")
		}
		wine, err := exec.LookPath("wine")
		if err != nil {
			return fmt.Errorf("o Wine do sistema não foi encontrado")
		}
		if game.PluginID != "linux" {
			options.Command = wine
			options.CommandArgs = []string{exe}
		}
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
	for dir := filepath.Dir(protonPath); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "steamapps" {
			return filepath.Dir(dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, candidate := range []string{
			filepath.Join(home, ".steam/root"),
			filepath.Join(home, ".local/share/Steam"),
			filepath.Join(home, ".steam/steam"),
		} {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
		return filepath.Join(home, ".steam/root")
	}
	return ""
}

// RunnerName devolve o nome legível de uma versão de Proton/Wine pelo caminho.
func RunnerName(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(filepath.Dir(path))
}
