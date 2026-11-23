import * as Host from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import type { LibrarySettings, MetadataSettings, StoreInfo } from "../bindings/github.com/playnux/core/app/models.js";

import { applyFontScale, DEFAULT_UI_SETTINGS, MAX_FONT_SCALE, MIN_FONT_SCALE, type UISettingsLike } from "./appearance";
import { createBackgroundDownloads } from "./background-downloads";
import { libraryPlugins, metadataPlugins } from "./plugins";
import { button, setSettingState, type SettingsProperty } from "./plugins/settings";
import { setupMetadataSettings } from "./plugins/metadata-settings";
import { language, setLanguage, translateStatic, t } from "./i18n";
import { storeLabel } from "./format";

type MetadataPluginID = keyof typeof metadataPlugins;
type Panel = "stores" | "library" | "plugins" | "appearance" | MetadataPluginID;

interface Options {
  stores: () => StoreInfo[];
  games: () => Game[];
  filteredGames: () => Game[];
  selected: () => Game | null;
  addGame: () => void;
  reload: (discoverNew?: boolean) => Promise<void>;
  applyDownloads: (games: Game[]) => void;
  error: (error: unknown) => string;
  recordGameError: (game: string, action: string, message: string) => void;
  report: (message: string, error?: boolean) => void;
  applyAppearance: (settings: UISettingsLike) => void;
  setViewMode: (mode: "details" | "grid") => void;
}

function element<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) throw new Error(t("Missing element {0}", id));
  return node as T;
}

const metadataSources = {
  store: t("Official game store"),
  igdb: "IGDB",
  steamgriddb: "SteamGridDB",
};
type MetadataSource = keyof typeof metadataSources;
const defaultPriority = Object.keys(metadataSources) as MetadataSource[];

export function setupIntegrations(options: Options) {
  const dialog = element<HTMLDialogElement>("integrations-dialog");
  const status = element("integrations-status");
  const metadataDialog = element<HTMLDialogElement>("metadata-dialog");
  const metadataStatus = element("metadata-status");
  const storeProperties = element("integration-store-properties");
  const storeActions = element("integration-store-actions");
  const storeName = element("integration-store-name");
  const storeSource = element("integration-store-source");
  const storeState = element("integration-store-state");
  const appearanceForm = element<HTMLFormElement>("appearance-form");
  const appearanceScale = element<HTMLInputElement>("appearance-font-size");
  const appearanceRange = element<HTMLInputElement>("appearance-font-range");
  const appearanceLanguage = element<HTMLSelectElement>("appearance-language");
  const appearanceStatus = element("appearance-status");
  const appearanceSave = element<HTMLButtonElement>("appearance-save");
  const syncLibraries = element<HTMLButtonElement>("sync-libraries");
  let appearanceRequest = 0;
  let savedFontScale = DEFAULT_UI_SETTINGS.font_scale;
  let savedGPU = DEFAULT_UI_SETTINGS.gpu_acceleration;
  let fontPreview = false;
  const storeList = element("integration-stores");
  const policyForm = element<HTMLFormElement>("metadata-policy");
  const sourceList = element("metadata-sources");
  const automatic = element<HTMLInputElement>("metadata-auto");
  const storeConfig = element("integration-store-config");
  const providerList = element("integration-providers");
  const content = element("integration-content");
  const providerStates = new Map<string, (settings: MetadataSettings) => void>();
  for (const [id, plugin] of Object.entries(metadataPlugins)) {
    content.insertAdjacentHTML("beforeend", plugin.view);
    translateStatic(content.lastElementChild!);
    providerStates.set(id, setupMetadataSettings(
      content.lastElementChild as HTMLElement,
      id as MetadataPluginID,
      { status, run: (task) => operation(task), error: options.error, report: options.report },
    ));
    const tab = button(plugin.name, () => showPanel(id as MetadataPluginID));
    tab.dataset.integrationTab = id;
    providerList.append(tab);
  }
  const panels = dialog.querySelectorAll<HTMLElement>("[data-integration-panel]");
  const tabs = dialog.querySelectorAll<HTMLButtonElement>("[data-integration-tab]");
  let busy = false;
  let currentPanel: Panel = "stores";
  let activeStore = "";
  let metadataGame: Game | null = null;
  let sourceOrder = [...defaultPriority];
  let activeSources = new Set<MetadataSource>(defaultPriority);
  let libraryPriority: string[] = [];
  const downloads = createBackgroundDownloads(options.error);

  function showPanel(panel: Panel): void {
    const enteringStores = panel === "stores" && (!dialog.open || currentPanel !== "stores");
    if (currentPanel === "appearance" && panel !== currentPanel) restoreFontPreview();
    currentPanel = panel;
    status.textContent = "";
    status.hidden = panel === "appearance";
    appearanceStatus.hidden = appearanceSave.hidden = panel !== "appearance";
    syncLibraries.hidden = panel !== "stores" && panel !== "library";
    renderStores(enteringStores);
    panels.forEach((section) => { section.hidden = section.dataset.integrationPanel !== panel; });
    tabs.forEach((tab) => {
      if (tab.dataset.integrationTab === panel) tab.setAttribute("aria-current", "page");
      else tab.removeAttribute("aria-current");
    });
    dialog.querySelectorAll<HTMLButtonElement>("[data-integration-panel-link]").forEach((link) => {
      if (link.dataset.integrationPanelLink === panel) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
    if (panel === "library") void renderLibrarySettings();
    if (panel === "appearance") void renderAppearance();
  }

  function renderLibraryPriority(): void {
    const list = element("library-priority");
    const names = new Map(options.stores().map((store) => [store.id, storeLabel(store)]));
    list.replaceChildren(...libraryPriority.map((id, index) => {
      const row = document.createElement("li");
      const label = document.createElement("span");
      label.textContent = names.get(id) ?? id;
      row.append(label);
      for (const [step, symbol, description] of [[-1, "↑", t("Move up")], [1, "↓", t("Move down")]] as const) {
        const target = index + step;
        const move = button(symbol, () => {
          [libraryPriority[index], libraryPriority[target]] = [libraryPriority[target], libraryPriority[index]];
          renderLibraryPriority();
        });
        move.setAttribute("aria-label", `${description} ${names.get(id) ?? id}`);
        move.disabled = target < 0 || target >= libraryPriority.length;
        row.append(move);
      }
      return row;
    }));
  }

  async function renderLibrarySettings(): Promise<void> {
    const statusLine = element("library-status");
    try {
      const settings: LibrarySettings = await Host.GetLibrarySettings();
      element<HTMLInputElement>("library-unify").checked = settings.unify;
      const known = new Set(options.stores().map((store) => store.id));
      libraryPriority = (settings.priority ?? []).filter((id) => known.has(id));
      for (const id of known) {
        if (!libraryPriority.includes(id)) libraryPriority.push(id);
      }
      renderLibraryPriority();
      statusLine.textContent = "";
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  async function saveLibrarySettings(): Promise<void> {
    const statusLine = element("library-status");
    statusLine.textContent = t("Saving…");
    try {
      await Host.SaveLibrarySettings({
        unify: element<HTMLInputElement>("library-unify").checked,
        priority: libraryPriority,
      });
      await options.reload(false);
      statusLine.textContent = t("Preferences saved.");
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  async function clearLibrary(): Promise<void> {
    const statusLine = element("library-status");
    if (!window.confirm(t("Clear the library? All imported games and local folders will be removed. Connected accounts keep their settings."))) {
      return;
    }
    statusLine.textContent = t("Clearing library…");
    try {
      await Host.ClearLibrary();
      await options.reload(false);
      await renderLibrarySettings();
      statusLine.textContent = t("Library cleared.");
    } catch (error) {
      statusLine.textContent = options.error(error);
    }
  }

  function renderSourceOrder(focusSource?: MetadataSource, focusStep?: number): void {
    sourceList.replaceChildren(...sourceOrder.map((source, index) => {
      const row = document.createElement("li");
      const label = document.createElement("label");
      const toggle = document.createElement("input");
      toggle.type = "checkbox";
      toggle.checked = activeSources.has(source);
      toggle.addEventListener("change", () => {
        if (toggle.checked) activeSources.add(source);
        else activeSources.delete(source);
      });
      label.append(toggle, metadataSources[source]);
      row.append(label);
      for (const [step, symbol, description] of [[-1, "↑", t("Move up")], [1, "↓", t("Move down")]] as const) {
        const target = index + step;
        const move = button(symbol, () => {
          [sourceOrder[index], sourceOrder[target]] = [sourceOrder[target], sourceOrder[index]];
          renderSourceOrder(source, step);
        });
        move.setAttribute("aria-label", `${description} ${metadataSources[source]}`);
        move.disabled = target < 0 || target >= sourceOrder.length;
        row.append(move);
        if (source === focusSource && step === focusStep) {
          queueMicrotask(() => (move.disabled ? toggle : move).focus());
        }
      }
      return row;
    }));
  }

  function renderPolicy(settings: MetadataSettings): void {
    automatic.checked = settings.policy.automatic;
    const saved = (settings.policy.sources ?? []).filter((id): id is MetadataSource => defaultPriority.includes(id as MetadataSource));
    activeSources = new Set(saved);
    sourceOrder = [...saved, ...defaultPriority.filter((source) => !activeSources.has(source))];
    const mode = settings.policy.mode === "replace" ? "replace" : "missing";
    policyForm.querySelector<HTMLInputElement>(`input[name="metadata-mode"][value="${mode}"]`)!.checked = true;
    renderSourceOrder();
  }

  function restoreFontPreview(): void {
    appearanceRequest++;
    if (fontPreview) {
      applyFontScale(savedFontScale);
      fontPreview = false;
    }
  }

  function previewFontScale(scale: number): void {
    if (!Number.isInteger(scale) || scale < MIN_FONT_SCALE || scale > MAX_FONT_SCALE) return;
    appearanceScale.value = appearanceRange.value = String(scale);
    applyFontScale(scale);
    fontPreview = scale !== savedFontScale;
  }

  async function renderAppearance(): Promise<void> {
    const request = ++appearanceRequest;
    appearanceForm.inert = true;
    appearanceSave.disabled = true;
    appearanceStatus.textContent = t("Loading…");
    try {
      const settings = await Host.GetUISettings();
      if (request !== appearanceRequest) return;
      savedFontScale = applyFontScale(settings.font_scale);
      savedGPU = settings.gpu_acceleration;
      fontPreview = false;
      appearanceScale.value = appearanceRange.value = String(savedFontScale);
      appearanceLanguage.value = language;
      element<HTMLSelectElement>("appearance-theme").value = settings.theme;
      element<HTMLSelectElement>("appearance-view").value = settings.view_mode;
      element<HTMLInputElement>("appearance-transitions").checked = !settings.transitions;
      element<HTMLInputElement>("appearance-gpu").checked = settings.gpu_acceleration;
      element<HTMLInputElement>("appearance-bg-trailer").checked = settings.background_trailer;
      element<HTMLInputElement>("appearance-bp-trailer").checked = settings.big_picture_trailer;
      appearanceStatus.textContent = "";
      appearanceSave.disabled = false;
    } catch (error) {
      if (request === appearanceRequest) appearanceStatus.textContent = options.error(error);
    } finally {
      if (request === appearanceRequest) appearanceForm.inert = false;
    }
  }

  const localImport = (id: string) => libraryPlugins[id]?.importsInstalled === true;

  async function operation(
    callback: () => Promise<void>,
    target = dialog,
    feedback = target === metadataDialog ? metadataStatus : status,
  ): Promise<void> {
    if (busy) return;
    busy = true;
    const controls = Array.from(target.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>(
      "button:not(.desktop-close), input, select",
    )).filter((control) => !control.inert);
    target.setAttribute("aria-busy", "true");
    controls.forEach((control) => { control.inert = true; });
    try {
      await callback();
    } catch (error) {
      feedback.textContent = options.error(error);
    } finally {
      busy = false;
      target.removeAttribute("aria-busy");
      controls.forEach((control) => { control.inert = false; });
    }
  }

  function renderStores(remount = false): void {
    const stores = options.stores();
    const selected = stores.find((store) => store.id === activeStore) ?? stores[0];
    activeStore = selected?.id ?? "";
    storeList.replaceChildren(...stores.map((store) => {
      const name = document.createElement("span");
      name.textContent = storeLabel(store);
      const item = button(storeLabel(store), () => {
        activeStore = store.id;
        showPanel("stores");
      });
      if (!libraryPlugins[store.id]?.local) {
        const indicator = document.createElement("span");
        indicator.className = "settings-nav-indicator";
        indicator.dataset.connected = String(store.connected);
        indicator.setAttribute("aria-hidden", "true");
        item.replaceChildren(name, indicator);
        item.title = store.connected ? t("Account connected") : t("Account not connected");
      }
      if (currentPanel === "stores" && store.id === activeStore) item.setAttribute("aria-current", "page");
      return item;
    }));
    storeName.textContent = selected ? storeLabel(selected) : t("Libraries");
    storeProperties.replaceChildren();
    storeActions.replaceChildren();
    storeProperties.hidden = true;
    storeState.hidden = false;
    if (!selected) {
      setSettingState(storeState, t("No integration available"), "unavailable");
      storeSource.textContent = "";
      storeConfig.replaceChildren();
      delete storeConfig.dataset.plugin;
      return;
    }
    const plugin = libraryPlugins[selected.id];
    const local = plugin?.local;
    storeSource.textContent = plugin?.source ?? t("Online library");
    setSettingState(storeState,
      local ? t("Local source") : selected.connected ? t("Account connected") : t("Account not connected"),
      local ? "local" : selected.connected ? "ready" : "pending",
    );
    const properties: SettingsProperty[] = [
      ...(plugin?.properties?.(selected) ?? []),
      ...(selected.clients ?? []).map((client): SettingsProperty => ({
        label: client.name,
        value: client.installed ? t("Detected") : t("Not installed"),
        state: client.installed ? "ready" : "unavailable",
      })),
    ];
    for (const property of properties) {
      const row = document.createElement("div");
      const term = document.createElement("dt");
      const definition = document.createElement("dd");
      term.textContent = property.label;
      if (property.state) setSettingState(definition, property.value, property.state);
      else definition.textContent = property.value;
      row.append(term, definition);
      storeProperties.append(row);
    }
    storeProperties.hidden = !properties.length;
    if (selected.connected) storeActions.append(button(t("Sync library"), () => queueLibrarySync([selected])));
    if (localImport(selected.id)) storeActions.append(button(t("Import installed games"), () => queueLibrarySync([selected], true)));
    if (currentPanel === "stores" && (remount || storeConfig.dataset.plugin !== selected.id)) {
      storeConfig.dataset.plugin = selected.id;
      storeConfig.innerHTML = plugin?.view ?? "";
      translateStatic(storeConfig);
      plugin?.setup?.(storeConfig, {
        store: selected,
        status,
        run: (task, feedback) => operation(task, dialog, feedback),
        reload: options.reload,
        sync: () => queueLibrarySync([{ ...selected, connected: true }]),
        error: options.error,
        report: options.report,
      });
    }
  }

  function queueLibrarySync(stores: StoreInfo[], installedOnly = false): void {
    const available = stores.filter((store) => store.connected || localImport(store.id));
    if (!available.length) {
      status.textContent = t("No library available to sync.");
      return;
    }
    dialog.close();
    downloads.enqueue(
      t("Libraries"),
      available,
      async (store) => {
        try {
          if (installedOnly || !store.connected) {
            await Host.ImportInstalledGames(store.id);
          } else {
            await Host.SyncStore(store.id);
          }
        } catch (error) {
          throw new Error(`${storeLabel(store)}: ${options.error(error)}`, { cause: error });
        }
      },
      () => options.reload(true),
    );
  }

  function open(panel: Panel, storeID?: string, message = ""): void {
    if (busy) return;
    if (panel === "plugins") {
      panel = (Object.keys(metadataPlugins)[0] as Panel) ?? "library";
    }
    if (storeID) activeStore = storeID;
    showPanel(panel);
    status.textContent = message;
    if (!dialog.open) dialog.showModal();
    void refreshProviders();
  }

  async function refreshProviders(): Promise<void> {
    try {
      const settings = await Host.GetMetadataSettings();
      for (const update of providerStates.values()) update(settings);
    } catch (error) {
      status.textContent = options.error(error);
    }
  }

  function openMetadata(game = options.selected()): void {
    if (busy || metadataDialog.open) return;
    metadataGame = game;
    const selection = policyForm.querySelector<HTMLInputElement>('[value="selected"]')!;
    selection.disabled = !game;
    selection.checked = !!game;
    if (!game) policyForm.querySelector<HTMLInputElement>('[value="all"]')!.checked = true;
    sourceList.replaceChildren();
    metadataStatus.textContent = t("Loading settings…");
    metadataDialog.showModal();
    void operation(async () => {
      renderPolicy(await Host.GetMetadataSettings());
      metadataStatus.textContent = "";
    }, metadataDialog);
  }

  async function savePolicy(): Promise<MetadataSettings["policy"]> {
    const sources = sourceOrder.filter((source) => activeSources.has(source));
    const mode = policyForm.querySelector<HTMLInputElement>('input[name="metadata-mode"]:checked')?.value ?? "missing";
    await Host.SaveMetadataPolicy(automatic.checked, sources, mode);
    return { automatic: automatic.checked, sources, mode };
  }

  function enqueueMetadata(ids: string[], policy: MetadataSettings["policy"]): void {
    const names = new Map(options.games().map((game) => [game.id, game.name]));
    downloads.enqueue(
      t("Metadata"),
      [...new Set(ids)],
      async (id) => {
        try {
          return await Host.DownloadGameMetadata(id, policy.sources, policy.mode);
        } catch (error) {
          options.recordGameError(names.get(id) || t("Library game"), t("Metadata"), options.error(error));
          throw error;
        }
      },
      (games) => options.applyDownloads(games.filter((game): game is Game => game !== null)),
    );
  }

  dialog.querySelectorAll<HTMLButtonElement>("[data-integration-panel-link]").forEach((link) => {
    link.addEventListener("click", () => showPanel((link.dataset.integrationPanelLink || "library") as Panel));
  });
  element("integration-metadata-settings").addEventListener("click", () => {
    dialog.close();
    openMetadata();
  });
  element("library-save").addEventListener("click", () => void saveLibrarySettings());
  element("library-add-game").addEventListener("click", () => {
    dialog.close();
    options.addGame();
  });
  element("library-clear").addEventListener("click", () => void clearLibrary());

  appearanceScale.addEventListener("input", () => previewFontScale(appearanceScale.valueAsNumber));
  appearanceRange.addEventListener("input", () => previewFontScale(appearanceRange.valueAsNumber));
  element("appearance-font-reset").addEventListener("click", () => previewFontScale(DEFAULT_UI_SETTINGS.font_scale));
  appearanceForm.addEventListener("submit", (event) => {
    event.preventDefault();
    void operation(async () => {
      const settings: UISettingsLike = {
        theme: element<HTMLSelectElement>("appearance-theme").value,
        font_scale: appearanceScale.valueAsNumber,
        view_mode: element<HTMLSelectElement>("appearance-view").value,
        transitions: !element<HTMLInputElement>("appearance-transitions").checked,
        gpu_acceleration: element<HTMLInputElement>("appearance-gpu").checked,
        background_trailer: element<HTMLInputElement>("appearance-bg-trailer").checked,
        big_picture_trailer: element<HTMLInputElement>("appearance-bp-trailer").checked,
      };
      appearanceStatus.textContent = t("Saving…");
      await Host.SaveUISettings(settings);
      if (appearanceLanguage.value !== language) {
        await Host.SetInterfaceLanguage(appearanceLanguage.value);
        setLanguage(appearanceLanguage.value);
        location.reload();
        return;
      }
      const changedGPU = savedGPU !== settings.gpu_acceleration;
      savedGPU = settings.gpu_acceleration;
      savedFontScale = settings.font_scale;
      fontPreview = false;
      options.applyAppearance(settings);
      options.setViewMode(settings.view_mode === "grid" ? "grid" : "details");
      appearanceStatus.textContent = changedGPU
        ? t("Appearance saved. Restart Playnux to apply the GPU setting.")
        : t("Appearance saved.");
    }, dialog, appearanceStatus);
  });

  dialog.addEventListener("close", restoreFontPreview);
  element("integrations-close").addEventListener("click", () => dialog.close());
  element("integrations-done").addEventListener("click", () => dialog.close());
  element("metadata-close").addEventListener("click", () => metadataDialog.close());
  element("metadata-done").addEventListener("click", () => metadataDialog.close());
  syncLibraries.addEventListener("click", () => queueLibrarySync(options.stores()));
  element("metadata-save").addEventListener("click", () => void operation(async () => {
    await savePolicy();
    metadataStatus.textContent = t("Metadata defaults saved.");
  }, metadataDialog));
  policyForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const scope = policyForm.querySelector<HTMLInputElement>('input[name="scope"]:checked')?.value;
    const games = scope === "selected" ? (metadataGame ? [metadataGame] : [])
      : scope === "filtered" ? options.filteredGames() : options.games();
    if (!games.length) {
      metadataStatus.textContent = t("No game selected.");
      return;
    }
    void operation(async () => {
      const policy = await savePolicy();
      metadataDialog.close();
      enqueueMetadata(games.map((game) => game.id), policy);
    }, metadataDialog);
  });

  return {
    open,
    openMetadata,
    refreshStores: () => { if (dialog.open) renderStores(); },
    downloadNew(ids: string[]) {
      if (!ids.length) return;
      void Host.GetMetadataSettings().then(({ policy }) => {
        if (policy.automatic) enqueueMetadata(ids, policy);
      }).catch((error) => options.report(options.error(error), true));
    },
  };
}
