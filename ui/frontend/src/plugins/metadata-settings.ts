import { t } from "../i18n";
import { Browser } from "@wailsio/runtime";
import * as Host from "../../bindings/github.com/playnux/ui/host/host.js";
import type { MetadataSettings } from "../../bindings/github.com/playnux/core/app/models.js";
import { setSettingState, type SettingsOperations } from "./settings";

export function setupMetadataSettings(
  root: HTMLElement,
  id: "igdb" | "steamgriddb",
  context: SettingsOperations,
): (settings: MetadataSettings) => void {
  const form = root.querySelector<HTMLFormElement>("form")!;
  const label = root.querySelector<HTMLElement>("[data-provider-state]")!;
  const help = root.querySelector<HTMLButtonElement>("[data-help-url]")!;

  function render(settings: MetadataSettings): void {
    const state = settings.saved_keys?.[id] ? t("Key saved")
      : settings[id] ? t("Environment key") : t("Missing key");
    setSettingState(label, state, settings[id] ? "ready" : "pending");
  }

  function update(clear: boolean): void {
    const data = new FormData(form);
    void context.run(async () => {
      await Host.SetMetadataCredentials(id, clear ? "" : String(data.get("client") ?? ""), clear ? "" : String(data.get("key") ?? ""));
      form.reset();
      render(await Host.GetMetadataSettings());
      context.status.textContent = clear ? t("Saved credentials removed.") : t("Credentials saved.");
    });
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    update(false);
  });
  form.querySelector("[data-clear-provider]")?.addEventListener("click", () => update(true));
  help.addEventListener("click", () => void context.run(async () => {
    if (help.dataset.helpUrl) await Browser.OpenURL(help.dataset.helpUrl);
  }));
  return render;
}
