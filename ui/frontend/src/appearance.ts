export interface UISettingsLike {
  theme: string;
  view_mode: string;
  gpu_acceleration: boolean;
  transitions: boolean;
}

export const DEFAULT_UI_SETTINGS: UISettingsLike = {
  theme: "theme-playnite-dark",
  view_mode: "details",
  gpu_acceleration: true,
  transitions: true,
};

export function applyUISettings(settings: UISettingsLike | null | undefined): UISettingsLike {
  const value: UISettingsLike = { ...DEFAULT_UI_SETTINGS, ...(settings || {}) };
  const body = document.body;
  body.dataset.theme = value.theme === "theme-steam" ? "theme-steam" : "theme-playnite-dark";
  body.classList.toggle("no-transitions", !value.transitions);
  body.classList.toggle("no-gpu", !value.gpu_acceleration);
  return value;
}
