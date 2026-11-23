// Package linux integra jogos nativos do Linux: entradas .desktop com a
// categoria Game, aplicativos Flatpak exportados e executáveis de pastas
// DRM-free indicadas pelo usuário.
package linux

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

// Provider é a fonte local de jogos do sistema. Ela não exige login; a lista
// de pastas DRM-free é configurada pelo Core e persistida no banco.
type Provider struct {
	mu sync.RWMutex

	folders     []string
	selection   []string
	desktopDirs []string
	flatpakDirs []string
}

func NewProvider() *Provider {
	home, _ := os.UserHomeDir()
	return &Provider{
		desktopDirs: []string{
			"/usr/share/applications",
			filepath.Join(home, ".local/share/applications"),
		},
		flatpakDirs: []string{
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
			"/var/lib/flatpak/exports/share/applications",
		},
	}
}

func (p *Provider) ID() string   { return "linux" }
func (p *Provider) Name() string { return "Linux / Flatpak" }

// IsLocal informa ao Core que esta fonte lê apenas o disco.
func (p *Provider) IsLocal() bool { return true }

func (p *Provider) Authenticate(context.Context, store.StoreAuth) error { return nil }
func (p *Provider) IsAuthenticated() bool                               { return true }

// SetFolders substitui as pastas DRM-free escaneadas.
func (p *Provider) SetFolders(folders []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.folders = cleanFolders(folders)
}

// Folders devolve uma cópia das pastas configuradas.
func (p *Provider) Folders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.folders...)
}

// SetSelection define quais aplicativos do sistema entram na biblioteca. A
// varredura de .desktop/Flatpak deixou de ser automática: o usuário escolhe os
// apps no diálogo Adicionar jogo.
func (p *Provider) SetSelection(ids []string) {
	cleaned := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleaned = append(cleaned, id)
	}
	sort.Strings(cleaned)
	p.mu.Lock()
	p.selection = cleaned
	p.mu.Unlock()
}

// Selection devolve os IDs escolhidos pelo usuário.
func (p *Provider) Selection() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.selection...)
}

// Candidates lista os aplicativos de jogos instalados no sistema, sem filtrar
// pela seleção salva. A UI mostra essa lista com caixas de seleção.
func (p *Provider) Candidates() []*models.Game {
	games := make(map[string]*models.Game)
	for _, dir := range p.desktopDirs {
		scanDesktopDir(dir, false, games)
	}
	for _, dir := range p.flatpakDirs {
		scanDesktopDir(dir, true, games)
	}
	result := make([]*models.Game, 0, len(games))
	for _, game := range games {
		result = append(result, game)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

// FetchLibrary combina os aplicativos escolhidos, exportações Flatpak e pastas
// locais. Entradas do usuário têm prioridade sobre as do sistema e exportações
// Flatpak substituem o .desktop genérico do mesmo aplicativo.
func (p *Provider) FetchLibrary(context.Context) ([]*models.Game, error) {
	games := make(map[string]*models.Game)

	p.mu.RLock()
	selected := make(map[string]bool, len(p.selection))
	for _, id := range p.selection {
		selected[id] = true
	}
	p.mu.RUnlock()

	for _, dir := range p.desktopDirs {
		scanDesktopDir(dir, false, games)
	}
	for _, dir := range p.flatpakDirs {
		scanDesktopDir(dir, true, games)
	}
	for key, game := range games {
		if !selected[game.GameID] {
			delete(games, key)
		}
	}

	p.mu.RLock()
	folders := append([]string(nil), p.folders...)
	p.mu.RUnlock()
	for _, folder := range folders {
		if isStoreLibraryPath(folder) {
			continue
		}
		for _, game := range scanGameFolder(folder) {
			games["folder:"+game.GameID] = game
		}
	}

	result := make([]*models.Game, 0, len(games))
	for _, game := range games {
		result = append(result, game)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

// LaunchCommand usa o comando já resolvido pelo scanner. Flatpaks são
// executados com "flatpak run <app_id>".
func (p *Provider) LaunchCommand(game *models.Game) (string, []string, error) {
	if game == nil {
		return "", nil, fmt.Errorf("linux: jogo inválido")
	}
	executable := strings.TrimSpace(game.Executable)
	if executable == "" {
		return "", nil, fmt.Errorf("linux: nenhum executável encontrado para %s", game.Name)
	}
	return executable, append([]string(nil), game.Arguments...), nil
}

func (p *Provider) UninstallCommand(*models.Game) (string, []string, error) {
	return "", nil, fmt.Errorf("linux: remova o jogo pelo gerenciador de pacotes ou apague a pasta")
}

// ── Entradas .desktop ───────────────────────────────────────────────────────

func scanDesktopDir(dir string, flatpakExport bool, games map[string]*models.Game) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
			continue
		}
		game := readDesktopGame(filepath.Join(dir, entry.Name()), flatpakExport)
		if game == nil || IsImportedLibraryTool(game) {
			continue
		}
		// Exportações Flatpak e o .desktop nativo do mesmo aplicativo aparecem
		// com nomes iguais. Mantém uma entrada só, preferindo a exportação Flatpak.
		key := "app:" + strings.ToLower(strings.TrimSpace(game.Name))
		if existing, ok := games[key]; ok && existing.Source == "Flatpak" && game.Source != "Flatpak" {
			continue
		}
		games[key] = game
	}
}

func readDesktopGame(path string, flatpakExport bool) *models.Game {
	values, err := parseDesktopFile(path)
	if err != nil {
		return nil
	}
	if truthy(values["Hidden"]) || truthy(values["NoDisplay"]) {
		return nil
	}
	categories := splitList(values["Categories"])
	if !hasCategory(categories, "Game") || isToolCategory(categories) {
		return nil
	}

	name := strings.TrimSpace(values["Name"])
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), ".desktop")
	}
	if isLibraryTool(name) {
		return nil
	}

	flatpakID := strings.TrimSpace(values["X-Flatpak"])
	if flatpakID == "" && flatpakExport {
		flatpakID = strings.TrimSuffix(filepath.Base(path), ".desktop")
	}
	if flatpakID != "" {
		return &models.Game{
			GameID:      flatpakID,
			PluginID:    "linux",
			Source:      "Flatpak",
			IsInstalled: true,
			Name:        name,
			SortingName: name,
			Description: strings.TrimSpace(values["Comment"]),
			Executable:  "flatpak",
			Arguments:   []string{"run", flatpakID},
			Icon:        strings.TrimSpace(values["Icon"]),
			Genres:      categoryNames(categories),
			Platforms:   []string{"PC (Linux)"},
			Tags:        []string{"Flatpak"},
		}
	}

	args := splitExec(values["Exec"])
	if len(args) == 0 {
		return nil
	}
	gameID := strings.TrimSuffix(filepath.Base(path), ".desktop")
	return &models.Game{
		GameID:      gameID,
		PluginID:    "linux",
		Source:      "Linux",
		IsInstalled: true,
		Name:        name,
		SortingName: name,
		Description: strings.TrimSpace(values["Comment"]),
		Executable:  args[0],
		Arguments:   args[1:],
		Icon:        strings.TrimSpace(values["Icon"]),
		Genres:      categoryNames(categories),
		Platforms:   []string{"PC (Linux)"},
		Tags:        []string{"Desktop"},
	}
}

func parseDesktopFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(file, 1<<20))
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if section != "Desktop Entry" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		// A UI em português usa Name[pt_BR], mas o nome sem locale é estável e
		// não depende do ambiente onde a biblioteca foi importada.
		if key == "Name" || key == "Exec" || key == "Icon" || key == "Comment" ||
			key == "Categories" || key == "Hidden" || key == "NoDisplay" || key == "X-Flatpak" {
			values[key] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// splitExec interpreta a linha Exec (aspas, escapes e placeholders do padrão
// XDG Desktop Entry) e devolve o binário seguido dos argumentos.
func splitExec(raw string) []string {
	var tokens []string
	var current strings.Builder
	quote := rune(0)
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()

	cleaned := tokens[:0]
	for _, token := range tokens {
		switch token {
		case "%f", "%F", "%u", "%U", "%i", "%c", "%k":
			continue
		}
		cleaned = append(cleaned, strings.ReplaceAll(token, "%%", "%"))
	}
	return cleaned
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

func splitList(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func hasCategory(categories []string, want string) bool {
	for _, category := range categories {
		if strings.EqualFold(category, want) {
			return true
		}
	}
	return false
}

// categoryNames remove metadados de classificação do menu e deixa apenas os
// gêneros que interessam à biblioteca.
func categoryNames(categories []string) []string {
	skip := map[string]bool{
		"game": true, "application": true, "kde": true, "qt": true,
		"gtk": true, "gnome": true, "x11": true, "consoleonly": true,
	}
	var result []string
	for _, category := range categories {
		if skip[strings.ToLower(category)] {
			continue
		}
		result = append(result, category)
	}
	return result
}

// ── Pastas DRM-free ─────────────────────────────────────────────────────────

type candidate struct {
	path  string
	name  string
	depth int
	group string
	dir   string
	score int
}

// scanGameFolder identifica executáveis ELF com permissão +x e scripts .sh.
// Cada subpasta de primeiro nível é tratada como um jogo; quando a própria
// pasta apontada é um jogo, todos os executáveis são agrupados nela.
func scanGameFolder(root string) []*models.Game {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}

	var candidates []candidate
	rootBase := normalizeName(filepath.Base(root))
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) > 5 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && ignoreDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || ignoreFile(entry.Name()) || isLibraryTool(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return nil
		}

		executable := isELF(path) || strings.EqualFold(filepath.Ext(path), ".sh")
		if !executable {
			return nil
		}

		name := normalizeName(executableName(entry.Name()))
		if name == "" {
			return nil
		}
		depth := len(parts)
		score := 20
		if name == rootBase {
			score = 0
		} else if name == normalizeName(filepath.Base(filepath.Dir(path))) {
			score = 5
		}
		if strings.EqualFold(filepath.Ext(path), ".sh") {
			score += 2
		}
		group := "file:" + entry.Name()
		dir := root
		if depth > 1 {
			group = "dir:" + parts[0]
			dir = filepath.Join(root, parts[0])
		}
		candidates = append(candidates, candidate{
			path: path, name: name, depth: depth, group: group, dir: dir, score: score,
		})
		return nil
	})

	if len(candidates) == 0 {
		return nil
	}

	// Pasta de um único jogo: agrupa tudo sob o nome da pasta.
	singleRoot := len(candidates) == 1
	for _, item := range candidates {
		if item.name == rootBase {
			singleRoot = true
			break
		}
	}

	groups := make(map[string][]candidate)
	for _, item := range candidates {
		key := item.group
		if singleRoot {
			key = "root"
		}
		groups[key] = append(groups[key], item)
	}

	games := make([]*models.Game, 0, len(groups))
	for key, items := range groups {
		sort.Slice(items, func(i, j int) bool {
			if items[i].score != items[j].score {
				return items[i].score < items[j].score
			}
			return len(items[i].path) < len(items[j].path)
		})
		chosen := items[0]

		name := chosen.name
		dir := chosen.dir
		if key == "root" || strings.HasPrefix(key, "dir:") {
			name = normalizeName(filepath.Base(dir))
		}
		if name == "" {
			name = chosen.name
		}
		games = append(games, &models.Game{
			GameID:           "local:" + chosen.path,
			PluginID:         "linux",
			Source:           "Local",
			Name:             name,
			SortingName:      name,
			Executable:       chosen.path,
			IsInstalled:      true,
			InstallDirectory: dir,
			Platforms:        []string{"PC (Linux)"},
			Tags:             []string{"DRM-free"},
		})
	}
	return games
}

func executableName(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		base = name
	}
	return base
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
	return strings.Join(strings.Fields(name), " ")
}

func ignoreDir(name string) bool {
	switch strings.ToLower(name) {
	case "lib", "lib64", "lib32", "share", "data", "redist", "redistributable",
		"directx", "vcredist", "dotnet", "mono", "support", "docs", "documentation",
		"locales", "resources", "node_modules", "__pycache__", ".git",
		"steam", ".steam", "steamapps", "steamlibrary", "steam-runtime",
		"epic games", "gog games", "heroic", "lutris":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// isStoreLibraryPath impede que uma pasta de loja (Steam/Epic/GOG/Heroic/Lutris)
// seja varrida como DRM-free. Sem isso, o mesmo jogo entrava duas vezes: uma
// pela loja e outra pela fonte local.
func isStoreLibraryPath(path string) bool {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	for _, part := range strings.Split(clean, "/") {
		switch strings.ToLower(part) {
		case "steam", ".steam", "steamapps", "steamlibrary", "steam-runtime",
			"epic games", "gog games", "heroic", "lutris":
			return true
		}
	}
	return strings.Contains(strings.ToLower(clean), "/steamapps/common/")
}

func ignoreFile(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "unins000.exe", "uninstall", "uninstaller", "setup", "install",
		"crashhandler", "crashpad_handler", "dxsetup", "vcredist_x64.exe":
		return true
	}
	return strings.Contains(lower, "uninstall") || strings.HasPrefix(lower, ".")
}

func isELF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return false
	}
	return string(header) == "\x7fELF"
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
