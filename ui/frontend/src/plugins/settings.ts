import * as Host from "../../bindings/github.com/playnux/ui/host/host.js";
import type { StoreInfo } from "../../bindings/github.com/playnux/core/app/models.js";

export interface SettingsOperations {
  status: HTMLElement;
  run: (task: () => Promise<void>, feedback?: HTMLElement) => Promise<void>;
  error: (error: unknown) => string;
  report: (message: string, error?: boolean) => void;
}

export interface SettingsContext extends SettingsOperations {
  store: StoreInfo;
  reload: (discoverNew?: boolean) => Promise<void>;
  sync: () => void;
}

export type SettingState = "ready" | "pending" | "local" | "unavailable";

export interface SettingsProperty {
  label: string;
  value: string;
  state?: SettingState;
}

export function setSettingState(element: HTMLElement, label: string, state: SettingState): void {
  element.classList.add("settings-state");
  element.dataset.state = state;
  element.textContent = label;
}

export function button(label: string, action: () => void): HTMLButtonElement {
  const control = document.createElement("button");
  control.type = "button";
  control.className = "ghost-button";
  control.textContent = label;
  control.addEventListener("click", action);
  return control;
}

export function bindDirectoryPicker(control: HTMLButtonElement, path: HTMLInputElement, context: SettingsOperations): void {
  control.addEventListener("click", () => void context.run(async () => {
    const selected = await Host.PickDirectory();
    if (selected) path.value = selected;
  }));
}
