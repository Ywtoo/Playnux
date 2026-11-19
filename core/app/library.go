package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
)

const (
	settingLocalSelection = "local.selected"
	settingLibraryUnify   = "library.unify"
	settingLibraryOrder   = "library.priority"
)

// defaultStorePriority é a ordem usada para escolher qual cópia representa um
// título unificado. O usuário pode reordenar isso nas configurações.
var defaultStorePriority = []string{"steam", "gog", "epic", "linux", "manual"}

// LibrarySettings são as preferências de exibição da biblioteca.
type LibrarySettings struct {
	Unify    bool     `json:"unify"`
	Priority []string `json:"priority"`
}

// LocalApp é um aplicativo instalado no sistema que o usuário pode adicionar.
type LocalApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Icon    string `json:"icon"`
	Added   bool   `json:"added"`
	Running bool   `json:"running"`
}

// LocalApps lista os apps de jogos detectados no sistema (desktop/Flatpak).
func (c *App) LocalApps(ctx context.Context) []LocalApp {
	if c.linux == nil {
		return nil
	}
	existing := make(map[string]bool)
	if games, err := c.db.ListGames(ctx, "all", 5000); err == nil {
		for _, game := range games {
			existing[game.GameID] = true
		}
	}
	apps := make([]LocalApp, 0)
	for _, game := range c.linux.Candidates() {
		apps = append(apps, LocalApp{
			ID:     game.GameID,
			Name:   game.Name,
			Source: game.Source,
			Icon:   game.Icon,
			Added:  existing[game.GameID],
		})
	}
	return apps
}

// AddLocalApps adiciona à biblioteca somente os aplicativos escolhidos.
func (c *App) AddLocalApps(ctx context.Context, ids []string) ([]*models.Game, error) {
	if c.linux == nil {
		return nil, errors.New("fonte de jogos Linux não registrada")
	}
	selection := append([]string(nil), c.linux.Selection()...)
	seen := make(map[string]bool, len(selection))
	for _, id := range selection {
		seen[id] = true
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		selection = append(selection, id)
	}
	if err := c.saveSelection(ctx, selection); err != nil {
		return nil, err
	}
	c.linux.SetSelection(selection)
	return c.manager.Sync(ctx, "linux")
}

func (c *App) saveSelection(ctx context.Context, selection []string) error {
	raw, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	return c.db.SetSetting(ctx, settingLocalSelection, string(raw))
}

func (c *App) loadSelection(ctx context.Context) error {
	if c.linux == nil {
		return nil
	}
	raw, err := c.db.GetSetting(ctx, settingLocalSelection)
	if errors.Is(err, database.ErrSettingNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var selection []string
	if err := json.Unmarshal([]byte(raw), &selection); err != nil {
		return nil
	}
	c.linux.SetSelection(selection)
	return nil
}

// ClearLibrary apaga todos os jogos importados, as pastas configuradas e a
// seleção de apps. Sessões e chaves de loja continuam salvas.
func (c *App) ClearLibrary(ctx context.Context) error {
	if err := c.db.ClearLibrary(ctx); err != nil {
		return err
	}
	if c.linux != nil {
		if err := c.saveFolders(ctx, settingLocalFolders, nil); err != nil {
			return err
		}
		c.linux.SetFolders(nil)
		if err := c.saveSelection(ctx, nil); err != nil {
			return err
		}
		c.linux.SetSelection(nil)
	}
	if c.emulation != nil {
		if err := c.saveFolders(ctx, settingROMFolders, nil); err != nil {
			return err
		}
		c.emulation.SetFolders(nil)
		if err := c.saveEmulationFolderProfiles(ctx, nil); err != nil {
			return err
		}
		c.emulation.SetFolderProfiles(nil)
	}
	return nil
}

// LibrarySettings lê as preferências de unificação.
func (c *App) LibrarySettings(context.Context) (LibrarySettings, error) {
	c.libraryMu.RLock()
	defer c.libraryMu.RUnlock()
	return LibrarySettings{Unify: c.library.Unify, Priority: append([]string(nil), c.library.Priority...)}, nil
}

// SaveLibrarySettings persiste a opção de unificar e a ordem de prioridade.
func (c *App) SaveLibrarySettings(ctx context.Context, settings LibrarySettings) error {
	priority := cleanPriority(settings.Priority)
	rawOrder, err := json.Marshal(priority)
	if err != nil {
		return err
	}
	rawUnify, err := json.Marshal(settings.Unify)
	if err != nil {
		return err
	}
	if err := c.db.SetSetting(ctx, settingLibraryOrder, string(rawOrder)); err != nil {
		return err
	}
	if err := c.db.SetSetting(ctx, settingLibraryUnify, string(rawUnify)); err != nil {
		return err
	}
	c.libraryMu.Lock()
	c.library = LibrarySettings{Unify: settings.Unify, Priority: priority}
	c.libraryMu.Unlock()
	return nil
}

func cleanPriority(priority []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(priority))
	for _, id := range priority {
		id = strings.TrimSpace(strings.ToLower(id))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	for _, id := range defaultStorePriority {
		if !seen[id] {
			result = append(result, id)
		}
	}
	return result
}

func (c *App) loadLibrarySettings(ctx context.Context) error {
	settings := LibrarySettings{Priority: append([]string(nil), defaultStorePriority...)}
	if raw, err := c.db.GetSetting(ctx, settingLibraryUnify); err == nil {
		_ = json.Unmarshal([]byte(raw), &settings.Unify)
	}
	if raw, err := c.db.GetSetting(ctx, settingLibraryOrder); err == nil {
		var priority []string
		if json.Unmarshal([]byte(raw), &priority) == nil && len(priority) > 0 {
			settings.Priority = cleanPriority(priority)
		}
	}
	c.libraryMu.Lock()
	c.library = settings
	c.libraryMu.Unlock()
	return nil
}

// CopyGameArtwork traz as imagens de outra cópia do jogo (outra loja) para a
// entrada exibida. É o "qual fica em destaque" das propriedades.
func (c *App) CopyGameArtwork(ctx context.Context, targetID, sourceID string) (*models.Game, error) {
	if targetID == sourceID {
		return c.GetGame(ctx, targetID)
	}
	source, err := c.db.GetGame(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{
		"icon":       source.Icon,
		"cover":      source.CoverImage,
		"background": source.BackgroundImage,
		"logo":       source.LogoImage,
	}
	for field, image := range fields {
		if strings.TrimSpace(image) == "" {
			continue
		}
		if err := c.db.SetGameArtwork(ctx, targetID, field, image); err != nil {
			return nil, err
		}
	}
	return c.GetGame(ctx, targetID)
}

// ── Unificação de títulos ───────────────────────────────────────────────────

func normalizedTitle(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	space := false
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			space = false
		case char >= 'à' && char <= 'ÿ':
			// Mantém letras acentuadas comuns em títulos em português.
			builder.WriteRune(char)
			space = false
		default:
			if !space && builder.Len() > 0 {
				builder.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func unifiablePlugin(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "steam", "gog", "epic", "linux", "manual":
		return true
	}
	return false
}

func (c *App) applyLibraryView(games []*models.Game) []*models.Game {
	c.libraryMu.RLock()
	settings := c.library
	c.libraryMu.RUnlock()
	if !settings.Unify {
		return games
	}

	groups := make(map[string][]*models.Game)
	order := make([]string, 0, len(games))
	for _, game := range games {
		if !unifiablePlugin(game.PluginID) {
			continue
		}
		key := normalizedTitle(game.Name)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], game)
	}

	merged := make(map[*models.Game]*models.Game, len(groups))
	used := make(map[*models.Game]bool, len(groups))
	for _, key := range order {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		representative := chooseRepresentative(group, settings.Priority)
		combined := mergeGames(representative, group)
		merged[representative] = combined
		for _, game := range group {
			used[game] = true
		}
	}

	result := make([]*models.Game, 0, len(games))
	for _, game := range games {
		if replacement, ok := merged[game]; ok {
			result = append(result, replacement)
			continue
		}
		if used[game] {
			continue
		}
		result = append(result, game)
	}
	return result
}

func chooseRepresentative(group []*models.Game, priority []string) *models.Game {
	rank := make(map[string]int, len(priority))
	for index, id := range priority {
		rank[id] = index
	}
	at := func(index int) int {
		if value, ok := rank[group[index].PluginID]; ok {
			return value
		}
		return len(priority) + 1
	}
	best := 0
	for index := 1; index < len(group); index++ {
		left, right := at(index), at(best)
		if left < right {
			best = index
			continue
		}
		if left == right && group[index].IsInstalled && !group[best].IsInstalled {
			best = index
		}
	}
	return group[best]
}

func mergeGames(representative *models.Game, group []*models.Game) *models.Game {
	combined := *representative
	variants := make([]models.GameVariant, 0, len(group))
	combined.IsInstalled = false
	for _, game := range group {
		combined.IsInstalled = combined.IsInstalled || game.IsInstalled
		if game.InstallSize > combined.InstallSize {
			combined.InstallSize = game.InstallSize
		}
		if game.Playtime > combined.Playtime {
			combined.Playtime = game.Playtime
		}
		if game.LastActivity != nil && (combined.LastActivity == nil || game.LastActivity.After(*combined.LastActivity)) {
			last := *game.LastActivity
			combined.LastActivity = &last
		}
		if combined.Description == "" {
			combined.Description = game.Description
		}
		if combined.CoverImage == "" {
			combined.CoverImage = game.CoverImage
		}
		if combined.BackgroundImage == "" {
			combined.BackgroundImage = game.BackgroundImage
		}
		if combined.LogoImage == "" {
			combined.LogoImage = game.LogoImage
		}
		if combined.Icon == "" {
			combined.Icon = game.Icon
		}
		if combined.TrailerURL == "" {
			combined.TrailerURL = game.TrailerURL
		}
		variants = append(variants, models.GameVariant{
			ID:              game.ID,
			Name:            game.Name,
			Source:          game.Source,
			PluginID:        game.PluginID,
			Icon:            game.Icon,
			CoverImage:      game.CoverImage,
			BackgroundImage: game.BackgroundImage,
			LogoImage:       game.LogoImage,
			TrailerURL:      game.TrailerURL,
			IsInstalled:     game.IsInstalled,
		})
	}
	sort.SliceStable(variants, func(i, j int) bool {
		return variants[i].Name < variants[j].Name
	})
	combined.Variants = variants
	return &combined
}

func (c *App) withVariants(ctx context.Context, game *models.Game) *models.Game {
	if game == nil {
		return nil
	}
	c.libraryMu.RLock()
	unify := c.library.Unify
	c.libraryMu.RUnlock()
	if !unify || !unifiablePlugin(game.PluginID) {
		return game
	}
	games, err := c.db.ListGames(ctx, "all", 5000)
	if err != nil {
		return game
	}
	key := normalizedTitle(game.Name)
	var group []*models.Game
	for _, candidate := range games {
		if unifiablePlugin(candidate.PluginID) && normalizedTitle(candidate.Name) == key {
			group = append(group, candidate)
		}
	}
	if len(group) < 2 {
		return game
	}
	for _, candidate := range group {
		if candidate.ID == game.ID {
			return mergeGames(game, group)
		}
	}
	return game
}
