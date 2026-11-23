import { t } from "../i18n";
import steam from "./steam/settings.html?raw";
import epic from "./epic/settings.html?raw";
import gog from "./gog/settings.html?raw";
import xbox from "./xbox/settings.html?raw";
import linux from "./linux/settings.html?raw";
import emulation from "./emulation/settings.html?raw";
import igdb from "./igdb/settings.html?raw";
import steamgriddb from "./steamgriddb/settings.html?raw";

import { properties as steamProperties, setup as setupSteam } from "./steam";
import { setup as setupLinux } from "./linux";
import { setup as setupEmulation } from "./emulation";
import { setupStoreLogin } from "./store-login";
import type { SettingsContext, SettingsProperty } from "./settings";

interface LibraryPluginSettings {
  view: string;
  source: string;
  importsInstalled?: boolean;
  local?: boolean;
  setup?: (root: HTMLElement, context: SettingsContext) => void;
  properties?: (store: SettingsContext["store"]) => SettingsProperty[];
}

export const libraryPlugins: Partial<Record<string, LibraryPluginSettings>> = {
  steam: {
    view: steam,
    source: t("Steam and local libraries"),
    importsInstalled: true,
    setup: setupSteam,
    properties: steamProperties,
  },
  epic: { view: epic, source: t("Epic Games, Heroic, and Legendary"), importsInstalled: true, setup: setupStoreLogin },
  gog: { view: gog, source: t("GOG and Heroic"), importsInstalled: true, setup: setupStoreLogin },
  xbox: { view: xbox, source: t("Microsoft account"), setup: setupStoreLogin },
  linux: { view: linux, source: t("Linux shortcuts, Flatpaks, and game folders"), local: true, setup: setupLinux },
  emulation: { view: emulation, source: t("ROM folders"), local: true, setup: setupEmulation },
};

export const metadataPlugins = {
  igdb: { name: "IGDB", view: igdb },
  steamgriddb: { name: "SteamGridDB", view: steamgriddb },
};
