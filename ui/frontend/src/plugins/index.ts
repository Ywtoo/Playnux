import steam from "./steam/settings.html?raw";
import epic from "./epic/settings.html?raw";
import gog from "./gog/settings.html?raw";
import xbox from "./xbox/settings.html?raw";
import linux from "./linux/settings.html?raw";
import emulation from "./emulation/settings.html?raw";
import igdb from "./igdb/settings.html?raw";
import steamgriddb from "./steamgriddb/settings.html?raw";

interface LibraryPluginSettings {
  view: string;
  source: string;
  importsInstalled?: boolean;
  local?: boolean;
}

export const libraryPlugins: Partial<Record<string, LibraryPluginSettings>> = {
  steam: { view: steam, source: "Steam e bibliotecas em disco", importsInstalled: true },
  epic: { view: epic, source: "Epic Games, Heroic e Legendary", importsInstalled: true },
  gog: { view: gog, source: "GOG e Heroic", importsInstalled: true },
  xbox: { view: xbox, source: "Conta Microsoft" },
  linux: { view: linux, source: "Atalhos Linux, Flatpaks e pastas de jogos", local: true },
  emulation: { view: emulation, source: "Pastas de ROMs", local: true },
};

export const metadataPlugins = {
  igdb: { name: "IGDB", view: igdb },
  steamgriddb: { name: "SteamGridDB", view: steamgriddb },
};
