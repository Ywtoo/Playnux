package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/playnux/core/database"
)

const (
	settingUITheme       = "ui.theme"
	settingUIView        = "ui.view_mode"
	settingUIGPU             = "ui.gpu_acceleration"
	settingUITransitions     = "ui.transitions"
	settingUIFontSize          = "ui.font_size"
	settingUIBackgroundTrailer = "ui.background_trailer"
	settingUIBigPictureTrailer = "ui.big_picture_trailer"

	ThemePlayniteDark = "theme-playnite-dark"
	ThemeSteam        = "theme-steam"
)

// UISettings holds appearance and performance preferences. The frontend applies
// theme and animation classes; the Core persists them in SQLite.
type UISettings struct {
	Theme             string `json:"theme"`
	ViewMode          string `json:"view_mode"`
	FontSize          string `json:"font_size"` // "small", "medium", "large"
	GPUAcceleration   bool   `json:"gpu_acceleration"`
	Transitions       bool   `json:"transitions"`
	BackgroundTrailer bool   `json:"background_trailer"`
	BigPictureTrailer bool   `json:"big_picture_trailer"`
}

// DefaultUISettings returns the visual defaults used when no preference exists.
func DefaultUISettings() UISettings {
	return UISettings{
		Theme:             ThemePlayniteDark,
		ViewMode:          "details",
		FontSize:          "medium",
		GPUAcceleration:   true,
		Transitions:       true,
		BackgroundTrailer: true,
		BigPictureTrailer: true,
	}
}

// UISettings reads the persisted appearance preferences.
func (c *App) UISettings(ctx context.Context) (UISettings, error) {
	settings := DefaultUISettings()
	if raw, err := c.db.GetSetting(ctx, settingUITheme); err == nil {
		settings.Theme = raw
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUIView); err == nil {
		settings.ViewMode = raw
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUIFontSize); err == nil {
		settings.FontSize = raw
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUIGPU); err == nil {
		_ = json.Unmarshal([]byte(raw), &settings.GPUAcceleration)
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUITransitions); err == nil {
		_ = json.Unmarshal([]byte(raw), &settings.Transitions)
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUIBackgroundTrailer); err == nil {
		_ = json.Unmarshal([]byte(raw), &settings.BackgroundTrailer)
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	if raw, err := c.db.GetSetting(ctx, settingUIBigPictureTrailer); err == nil {
		_ = json.Unmarshal([]byte(raw), &settings.BigPictureTrailer)
	} else if !errors.Is(err, database.ErrSettingNotFound) {
		return settings, err
	}
	return NormalizeUISettings(settings), nil
}

// SaveUISettings validates and persists all appearance preferences.
func (c *App) SaveUISettings(ctx context.Context, settings UISettings) error {
	settings = NormalizeUISettings(settings)
	gpu, err := json.Marshal(settings.GPUAcceleration)
	if err != nil {
		return err
	}
	transitions, err := json.Marshal(settings.Transitions)
	if err != nil {
		return err
	}
	bgTrailer, err := json.Marshal(settings.BackgroundTrailer)
	if err != nil {
		return err
	}
	bpTrailer, err := json.Marshal(settings.BigPictureTrailer)
	if err != nil {
		return err
	}
	values := map[string]string{
		settingUITheme:             settings.Theme,
		settingUIView:              settings.ViewMode,
		settingUIFontSize:          settings.FontSize,
		settingUIGPU:               string(gpu),
		settingUITransitions:       string(transitions),
		settingUIBackgroundTrailer: string(bgTrailer),
		settingUIBigPictureTrailer: string(bpTrailer),
	}
	for key, value := range values {
		if err := c.db.SetSetting(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeUISettings clamps unknown themes or view modes to safe defaults.
func NormalizeUISettings(settings UISettings) UISettings {
	theme := strings.TrimSpace(settings.Theme)
	if theme != ThemeSteam && theme != ThemePlayniteDark {
		theme = ThemePlayniteDark
	}
	view := strings.TrimSpace(settings.ViewMode)
	if view != "grid" && view != "details" {
		view = "details"
	}
	fontSize := strings.TrimSpace(settings.FontSize)
	if fontSize != "small" && fontSize != "medium" && fontSize != "large" {
		fontSize = "medium"
	}
	return UISettings{
		Theme:             theme,
		ViewMode:          view,
		FontSize:          fontSize,
		GPUAcceleration:   settings.GPUAcceleration,
		Transitions:       settings.Transitions,
		BackgroundTrailer: settings.BackgroundTrailer,
		BigPictureTrailer: settings.BigPictureTrailer,
	}
}

// ValidateTheme is exposed for tests and other Core surfaces.
func ValidateTheme(theme string) error {
	if theme != ThemeSteam && theme != ThemePlayniteDark {
		return fmt.Errorf("unknown theme %q", theme)
	}
	return nil
}
