package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/playnux/core/database"
	"github.com/playnux/core/models"
	"github.com/playnux/core/store/builtins/emulation"
)

const (
	settingEmulationProfiles = "emulation.folder_profiles"
)

// loadEmulation loads custom emulators and folder-to-profile mappings into the
// provider. Detection itself stays lazy so startup is not blocked by Flatpak.
func (c *App) loadEmulation(ctx context.Context) error {
	if c.emulation == nil {
		return nil
	}
	records, err := c.db.ListCustomEmulators(ctx)
	if err != nil {
		return err
	}
	definitions := make([]emulation.EmulatorDefinition, 0, len(records))
	for _, record := range records {
		var profiles []emulation.EmulatorProfile
		if record.ProfilesJSON != "" {
			_ = json.Unmarshal([]byte(record.ProfilesJSON), &profiles)
		}
		definitions = append(definitions, emulation.EmulatorDefinition{
			ID:         record.ID,
			Name:       record.Name,
			Executable: record.Executable,
			FlatpakID:  record.FlatpakID,
			Profiles:   profiles,
			Custom:     true,
		})
	}
	c.emulation.SetCustom(definitions)
	mapping, err := c.emulationFolderProfiles(ctx)
	if err != nil {
		return err
	}
	c.emulation.SetFolderProfiles(mapping)
	return nil
}

// Emulators lists detected emulators plus the user-defined catalog.
func (c *App) Emulators(context.Context) []emulation.EmulatorDefinition {
	if c.emulation == nil {
		return nil
	}
	return c.emulation.Catalog()
}

// SaveEmulator validates and persists a custom emulator definition.
func (c *App) SaveEmulator(ctx context.Context, input emulation.CustomEmulator) error {
	if c.emulation == nil {
		return fmt.Errorf("emulation provider is not registered")
	}
	if emulation.IsBuiltinEmulator(input.ID) {
		return fmt.Errorf("this emulator is built in and cannot be overwritten")
	}
	normalized, err := emulation.NormalizeCustom(emulation.EmulatorDefinition{
		ID:         input.ID,
		Name:       input.Name,
		Executable: input.Executable,
		FlatpakID:  input.FlatpakID,
		Profiles:   input.Profiles,
	})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized.Profiles)
	if err != nil {
		return err
	}
	if err := c.db.SaveCustomEmulator(ctx, database.EmulatorRecord{
		ID:           normalized.ID,
		Name:         normalized.Name,
		Executable:   normalized.Executable,
		FlatpakID:    normalized.FlatpakID,
		ProfilesJSON: string(raw),
	}); err != nil {
		return err
	}
	return c.loadEmulation(ctx)
}

// RemoveEmulator deletes a custom emulator and its folder mappings.
func (c *App) RemoveEmulator(ctx context.Context, id string) error {
	if c.emulation == nil {
		return fmt.Errorf("emulation provider is not registered")
	}
	id = strings.TrimSpace(id)
	if id == "" || emulation.IsBuiltinEmulator(id) {
		return fmt.Errorf("only custom emulators can be removed")
	}
	if err := c.db.DeleteCustomEmulator(ctx, id); err != nil {
		return err
	}
	mapping, err := c.emulationFolderProfiles(ctx)
	if err != nil {
		return err
	}
	for folder, profileID := range mapping {
		if strings.HasPrefix(profileID, id+":") || profileID == id {
			delete(mapping, folder)
		}
	}
	if err := c.saveEmulationFolderProfiles(ctx, mapping); err != nil {
		return err
	}
	c.emulation.SetFolderProfiles(mapping)
	return c.loadEmulation(ctx)
}

// EmulationFolderProfiles exposes the folder-to-profile mapping for Settings.
func (c *App) EmulationFolderProfiles() map[string]string {
	if c.emulation == nil {
		return nil
	}
	return c.emulation.FolderProfiles()
}

// AddEmulationFolder scans a ROM folder with a chosen profile and remembers the
// choice for future synchronisations.
func (c *App) AddEmulationFolder(ctx context.Context, path, profileID string) ([]*models.Game, error) {
	if c.emulation == nil {
		return nil, fmt.Errorf("emulation provider is not registered")
	}
	folder, err := normalizeFolder(path)
	if err != nil {
		return nil, err
	}
	if profileID != "" {
		profile, found := emulation.FindProfile(c.emulation.Catalog(), profileID)
		if !found {
			return nil, fmt.Errorf("emulator profile not found")
		}
		if profile.Executable == "" && profile.FlatpakID == "" {
			return nil, fmt.Errorf("configure the emulator executable before scanning")
		}
	}

	mapping, err := c.emulationFolderProfiles(ctx)
	if err != nil {
		return nil, err
	}
	if profileID == "" {
		delete(mapping, folder)
	} else {
		mapping[folder] = profileID
	}
	if err := c.saveEmulationFolderProfiles(ctx, mapping); err != nil {
		return nil, err
	}
	c.emulation.SetFolderProfiles(mapping)

	folders := c.emulation.Folders()
	found := false
	for _, existing := range folders {
		if existing == folder {
			found = true
			break
		}
	}
	if !found {
		folders = append(folders, folder)
		sort.Strings(folders)
		if err := c.saveFolders(ctx, settingROMFolders, folders); err != nil {
			return nil, err
		}
		c.emulation.SetFolders(folders)
	}
	return c.manager.Sync(ctx, "emulation")
}

func (c *App) emulationFolderProfiles(ctx context.Context) (map[string]string, error) {
	raw, err := c.db.GetSetting(ctx, settingEmulationProfiles)
	if err != nil {
		if err == database.ErrSettingNotFound {
			return make(map[string]string), nil
		}
		return nil, err
	}
	mapping := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
		return make(map[string]string), nil
	}
	return mapping, nil
}

func (c *App) saveEmulationFolderProfiles(ctx context.Context, mapping map[string]string) error {
	if mapping == nil {
		mapping = make(map[string]string)
	}
	raw, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	return c.db.SetSetting(ctx, settingEmulationProfiles, string(raw))
}

// ValidateEmulationProfile checks a profile without persisting anything.
func ValidateEmulationProfile(profile emulation.EmulatorProfile) error {
	if strings.TrimSpace(profile.Executable) == "" && strings.TrimSpace(profile.FlatpakID) == "" {
		return fmt.Errorf("inform an emulator executable or Flatpak ID")
	}
	if len(profile.Extensions) == 0 {
		return fmt.Errorf("inform at least one ROM extension")
	}
	for _, extension := range profile.Extensions {
		if strings.TrimSpace(extension) == "" {
			return fmt.Errorf("empty ROM extension")
		}
	}
	return nil
}
