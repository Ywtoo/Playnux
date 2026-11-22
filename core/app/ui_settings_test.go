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

	want := UISettings{Theme: ThemeSteam, ViewMode: "grid", FontSize: "large", GPUAcceleration: false, Transitions: false}
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
	got := NormalizeUISettings(UISettings{Theme: "neon", ViewMode: "list", FontSize: "huge"})
	if got.Theme != ThemePlayniteDark || got.ViewMode != "details" || got.FontSize != "medium" {
		t.Fatalf("normalized = %+v", got)
	}
	if err := ValidateTheme("neon"); err == nil {
		t.Fatal("expected an unknown theme error")
	}
}
