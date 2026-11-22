export interface UISettingsLike {
  theme: string;
  view_mode: string;
  font_size: string;
  gpu_acceleration: boolean;
  transitions: boolean;
  background_trailer: boolean;
  big_picture_trailer: boolean;
}

export const DEFAULT_UI_SETTINGS: UISettingsLike = {
  theme: "theme-playnite-dark",
  view_mode: "details",
  font_size: "medium",
  gpu_acceleration: true,
  transitions: true,
  background_trailer: true,
  big_picture_trailer: true,
};

export function applyUISettings(settings: UISettingsLike | null | undefined): UISettingsLike {
  const value: UISettingsLike = { ...DEFAULT_UI_SETTINGS, ...(settings || {}) };
  const body = document.body;
  body.dataset.theme = value.theme === "theme-steam" ? "theme-steam" : "theme-playnite-dark";
  body.dataset.fontSize = value.font_size || "medium";
  body.classList.toggle("no-transitions", !value.transitions);
  body.classList.toggle("no-gpu", !value.gpu_acceleration);
  return value;
}
