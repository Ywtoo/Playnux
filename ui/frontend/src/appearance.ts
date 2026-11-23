export interface UISettingsLike {
  theme: string;
  view_mode: string;
  font_scale: number;
  gpu_acceleration: boolean;
  transitions: boolean;
  background_trailer: boolean;
  big_picture_trailer: boolean;
}

export const DEFAULT_UI_SETTINGS: UISettingsLike = {
  theme: "theme-playnite-dark",
  view_mode: "details",
  font_scale: 100,
  gpu_acceleration: true,
  transitions: true,
  background_trailer: true,
  big_picture_trailer: true,
};

export const MIN_FONT_SCALE = 80;
export const MAX_FONT_SCALE = 160;

export function applyFontScale(scale: number): number {
  const value = Number.isFinite(scale) && scale > 0
    ? Math.min(MAX_FONT_SCALE, Math.max(MIN_FONT_SCALE, Math.round(scale)))
    : DEFAULT_UI_SETTINGS.font_scale;
  document.documentElement.style.setProperty("--ui-text-scale", String(value / 100));
  return value;
}

export function applyUISettings(settings: UISettingsLike | null | undefined): UISettingsLike {
  const value: UISettingsLike = { ...DEFAULT_UI_SETTINGS, ...(settings || {}) };
  const body = document.body;
  body.dataset.theme = value.theme === "theme-steam" ? "theme-steam" : "theme-playnite-dark";
  value.font_scale = applyFontScale(value.font_scale);
  body.classList.toggle("no-transitions", !value.transitions);
  body.classList.toggle("no-gpu", !value.gpu_acceleration);
  return value;
}
