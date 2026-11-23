import { Browser } from "@wailsio/runtime";
import { t } from "../../i18n";
import * as Host from "../../../bindings/github.com/playnux/ui/host/host.js";
import type { SettingsContext, SettingsProperty } from "../settings";

export function setup(root: HTMLElement, context: SettingsContext): void {
  const account = root.querySelector<HTMLFormElement>("[data-steam-account]");
  const steamID = root.querySelector<HTMLInputElement>("#steam-id");
  const detectSteam = root.querySelector<HTMLButtonElement>("[data-steam-detect]");
  root.querySelector("[data-steam-api-page]")?.addEventListener("click", () => {
    void Browser.OpenURL("https://steamcommunity.com/dev/apikey").catch((error) => {
      context.report(context.error(error), true);
    });
  });
  if (steamID) steamID.value = context.store.account_id ?? "";
  if (steamID && !steamID.value) {
    void Host.DetectSteamID().then((id) => {
      if (id && !steamID.value) steamID.value = id;
    }).catch(() => undefined);
  }
  detectSteam?.addEventListener("click", () => {
    detectSteam.disabled = true;
    void Host.DetectSteamID().then((id) => {
      if (steamID) steamID.value = id;
      context.status.textContent = t("SteamID64 found on this PC: {0}", id);
    }).catch((error) => {
      context.status.textContent = context.error(error);
    }).finally(() => {
      detectSteam.disabled = false;
    });
  });
  account?.addEventListener("submit", (event) => {
    event.preventDefault();
    const data = new FormData(account);
    void context.run(async () => {
      const key = String(data.get("key") ?? "").trim();
      const clearKey = data.has("clearKey");
      await Host.ConfigureSteam(String(data.get("account") ?? ""), key, clearKey);
      account.reset();
      if (!key && !clearKey) {
        context.report(t("SteamID saved and installed games imported. Add a Steam Web API key to load your uninstalled purchases."));
      }
      context.sync();
    });
  });
}

export function properties(store: SettingsContext["store"]): SettingsProperty[] {
  return [{
    label: t("Steam Web API key"),
    value: store.api_key_saved ? t("Saved") : t("Pending"),
    state: store.api_key_saved ? "ready" : "pending",
  }];
}
