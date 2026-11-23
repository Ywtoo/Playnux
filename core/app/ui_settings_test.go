package app

import (
	"context"
	"testing"
)

func TestUISettingsRoundTrip(t *testing.T) {
	core, err := NewWithProviders(":memory:", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()

	defaults, err := core.UISettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Theme != ThemePlayniteDark || !defaults.Transitions || !defaults.GPUAcceleration {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	want := UISettings{Theme: ThemeSteam, ViewMode: "grid", FontScale: 137, GPUAcceleration: false, Transitions: false}
	if err := core.SaveUISettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := core.UISettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}

func TestUISettingsClampsUnknownValues(t *testing.T) {
	got := NormalizeUISettings(UISettings{Theme: "neon", ViewMode: "list", FontScale: 210})
	if got.Theme != ThemePlayniteDark || got.ViewMode != "details" || got.FontScale != 160 {
		t.Fatalf("normalized = %+v", got)
	}
	if err := ValidateTheme("neon"); err == nil {
		t.Fatal("expected an unknown theme error")
	}
}

func TestUISettingsReadsLegacyFontSizes(t *testing.T) {
	core, err := NewWithProviders(":memory:", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()

	for legacy, expected := range map[string]int{"small": 88, "medium": 100, "large": 112} {
		if err := core.db.SetSetting(ctx, settingUIFontSize, legacy); err != nil {
			t.Fatal(err)
		}
		got, err := core.UISettings(ctx)
		if err != nil || got.FontScale != expected {
			t.Fatalf("escala de %s = %d, erro = %v", legacy, got.FontScale, err)
		}
	}
}
