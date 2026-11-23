import { Events } from "@wailsio/runtime";
import * as HostService from "../bindings/github.com/playnux/ui/host/host.js";
import type { Game } from "../bindings/github.com/playnux/core/models/models.js";
import type { LocalApp, StoreInfo } from "../bindings/github.com/playnux/core/app/models.js";
import "./components/game-card";
import type { PlaynuxGameCardElement } from "./components/game-card";
import { applyUISettings, DEFAULT_UI_SETTINGS, type UISettingsLike } from "./appearance";
import { createBigPicture } from "./big-picture";
import { renderCompatibility } from "./compatibility";
import { setupIntegrations } from "./integrations";
import { installDeepSleep } from "./deep-sleep";
import {
  canLaunch,
  formatPlaytime,
  formatSize,
  initials,
  launchBlockReason,
  sourceLabel,
} from "./format";
import { setupGameProperties } from "./game-properties";
import { renderGameInformation } from "./game-information";
import { backgroundTrailer } from "./media";
import { sampleBackdrop, setArtworkImage } from "./artwork-images";
import { setupNotifications } from "./notifications";
import { configureDetailsSidebar, configureGridCards } from "./library-layout";
import { language, translateStatic, t } from "./i18n";
import { setupUpdates } from "./updates";

document.documentElement.lang = language;
translateStatic(document.body);

const FILTER_TITLES: Record<string, string> = {
  all: t("All games"),
  installed: t("Installed"),
  favorites: t("Favorites"),
  recent: t("Recent"),
};

type ViewMode = "details" | "grid";
type EmptyMode = "refresh" | "clear" | "all" | "login" | "manual";

function element<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (!found) {
    throw new Error(t("element #{0} is missing", id));
  }
  return found as T;
}

const recordGameError = setupNotifications(
  element<HTMLDialogElement>("notifications-dialog"),
  element<HTMLButtonElement>("notifications-open"),
  element("notifications-count"),
  element("notifications-list"),
  element<HTMLButtonElement>("notifications-clear"),
  element<HTMLButtonElement>("notifications-close"),
);

const searchInput = element<HTMLInputElement>("search");
const bigPictureButton = element<HTMLButtonElement>("big-picture-open");
const viewModeButtons = Array.from(document.querySelectorAll<HTMLButtonElement>(".view-mode"));

const detailsView = element<HTMLElement>("details-view");
const sidebarToggle = element<HTMLButtonElement>("details-sidebar-toggle");
const gridView = element<HTMLElement>("grid-view");
const detailsList = element<HTMLElement>("details-list");
const detailsCount = element<HTMLElement>("details-count");
const detailsHero = element<HTMLElement>("details-hero");
const detailsPlaceholder = element<HTMLElement>("details-placeholder");
const detailsBody = element<HTMLElement>("details-body");
const detailsBg = element<HTMLImageElement>("details-bg");
const detailsLogo = element<HTMLImageElement>("details-logo");
const detailsTitle = element<HTMLElement>("details-title");
const detailsPlay = element<HTMLButtonElement>("details-play");
const detailsTrailer = backgroundTrailer(element("details-hero-video"), element<HTMLButtonElement>("details-trailer-sound"));
const detailsDescription = element<HTMLElement>("details-description");
const detailsFacts = element<HTMLElement>("details-facts");
const detailsCompatibility = element<HTMLElement>("details-compatibility");

const gameGrid = element<HTMLElement>("game-grid");
const empty = element<HTMLElement>("empty");
const emptyTitle = element<HTMLElement>("empty-title");
const emptyText = element<HTMLElement>("empty-text");
const emptyAction = element<HTMLButtonElement>("empty-action");
const skeleton = element<HTMLElement>("skeleton");
const sourceFilter = element<HTMLSelectElement>("source-filter");
const sectionTitle = element<HTMLElement>("section-title");
const gameCount = element<HTMLElement>("game-count");
const gridZoom = element<HTMLInputElement>("grid-zoom");

const drawer = element<HTMLElement>("drawer");
const drawerClose = document.querySelector<HTMLButtonElement>(".drawer-close");
const drawerCover = element<HTMLImageElement>("drawer-cover");
const drawerInitial = element<HTMLElement>("drawer-initial");
const drawerSource = element<HTMLElement>("drawer-source");
const drawerTitle = element<HTMLElement>("drawer-title");
const drawerPlay = element<HTMLButtonElement>("drawer-play");
const drawerInstalled = element<HTMLElement>("drawer-installed");
const drawerDescription = element<HTMLElement>("drawer-description");
const drawerFacts = element<HTMLElement>("drawer-facts");
const drawerTrailer = backgroundTrailer(element("drawer-trailer-background"), element<HTMLButtonElement>("drawer-trailer-sound"));
const drawerCompatibility = element<HTMLElement>("drawer-compatibility");


const manualDialog = element<HTMLDialogElement>("manual-dialog");
const manualForm = element<HTMLFormElement>("manual-form");
const manualPath = element<HTMLInputElement>("manual-path");
const manualName = element<HTMLInputElement>("manual-name");
const manualPick = element<HTMLButtonElement>("manual-pick");
const manualCancel = element<HTMLButtonElement>("manual-cancel");
const manualSubmit = element<HTMLButtonElement>("manual-submit");
const manualScanFolder = element<HTMLButtonElement>("manual-scan-folder");
const manualError = element<HTMLElement>("manual-error");

const appsSearch = element<HTMLInputElement>("apps-search");
const appsRefresh = element<HTMLButtonElement>("apps-refresh");
const appsList = element<HTMLElement>("apps-list");
const appsAdd = element<HTMLButtonElement>("apps-add");

const bigPictureRoot = element<HTMLElement>("big-picture");
const filterButtons = Array.from(document.querySelectorAll<HTMLButtonElement>(".top-filter-btn"));
const toast = element<HTMLElement>("toast");

let allGames: Game[] = [];
let visibleGames: Game[] = [];
let stores: StoreInfo[] = [];
let activeFilter = "all";
let searchQuery = "";
let selectedGame: Game | null = null;
let detailsGame: Game | null = null;
let emptyMode: EmptyMode = "refresh";
let emptyStoreID = "";
let launching = false;
let manualBusy = false;
let localApps: LocalApp[] = [];
let selectedApps = new Set<string>();
let appsLoading = false;
let loadSequence = 0;
let searchTimer = 0;
let toastTimer = 0;
let gridDirty = true;
let lastFocused: HTMLElement | null = null;

function readViewMode(): ViewMode {
  const stored = localStorage.getItem("playnux:view");
  return stored === "grid" ? "grid" : "details";
}

let viewMode: ViewMode = readViewMode();
let uiSettings: UISettingsLike = { ...DEFAULT_UI_SETTINGS, view_mode: viewMode };

const bigPicture = createBigPicture({
  root: bigPictureRoot,
  trailersEnabled: () => uiSettings.big_picture_trailer,
  onExit: refreshBackgroundTrailers,
  onError: (error) => showToast(t("Could not restore the window. {0}", messageOf(error)), true),
  onLaunch: (game) => {
    void launchGame(game, "big");
  },
});

function cleanGames(games: (Game | null)[] | null | undefined): Game[] {
  return (games || []).filter((game): game is Game => game !== null && typeof game === "object");
}

function remoteImage(value: string | null | undefined): string {
  const source = (value || "").trim();
  return /^(https?:|data:|blob:|\/artwork\/)/i.test(source) ? source : "";
}

function messageOf(error: unknown): string {
  let message = "";
  if (typeof error === "string") {
    message = error;
  } else if (error instanceof Error) {
    message = error.message;
  } else if (error && typeof error === "object" && "message" in error) {
    message = String((error as { message: unknown }).message);
  }
  const cleaned = message.replace(/^(RuntimeError|Error):\s*/i, "").trim();
  return cleaned || t("Could not complete the operation.");
}

function showToast(message: string, isError = false): void {
  toast.textContent = message;
  toast.classList.toggle("is-error", isError);
  toast.classList.add("is-visible");
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    toast.classList.remove("is-visible");
  }, isError ? 7000 : 4200);
}

// ── Biblioteca ──────────────────────────────────────────────────────────────

function updateCounts(): void {
  const counts: Record<string, number> = {
    all: allGames.length,
    installed: 0,
    favorites: 0,
    recent: 0,
  };
  for (const game of allGames) {
    if (game.is_installed) {
      counts.installed++;
    }
    if (game.favorite) {
      counts.favorites++;
    }
    if (game.last_activity) {
      counts.recent++;
    }
  }
  document.querySelectorAll<HTMLElement>("[data-count]").forEach((item) => {
    const key = item.dataset.count || "";
    item.textContent = String(counts[key] || 0);
  });
}

function updateSectionTitle(): void {
  const title = searchQuery.trim() ? t("Search results") : FILTER_TITLES[activeFilter] || t("Library");
  const origin = sourceFilter.value ? sourceFilter.selectedOptions[0]?.label : "";
  sectionTitle.textContent = origin ? `${title} · ${origin}` : title;
}

function updateFilterButtons(): void {
  for (const button of filterButtons) {
    const active = (button.dataset.filter || "all") === activeFilter;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  }
}

function updateEmpty(games: Game[]): void {
  if (games.length > 0) {
    empty.hidden = true;
    return;
  }
  empty.hidden = false;
  if (searchQuery.trim()) {
    emptyMode = "clear";
    emptyTitle.textContent = t("Nothing found");
    emptyText.textContent = t("No game matches “{0}”. Try another search.", searchQuery.trim());
    emptyAction.textContent = t("Clear search");
    return;
  }
  if (activeFilter !== "all" || sourceFilter.value) {
    emptyMode = "all";
    emptyTitle.textContent = t("No games in this category");
    emptyText.textContent = t("No games match this filter. View the entire library or add a game.");
    emptyAction.textContent = t("View all games");
    return;
  }
  const disconnected = stores.find((store) => !store.connected && store.id !== "xbox");
  if (disconnected && allGames.length === 0) {
    emptyMode = "login";
    emptyStoreID = disconnected.id;
    emptyTitle.textContent = t("Connect your first store");
    emptyText.textContent = t("Sign in to {0} or import installed games without signing in.", disconnected.name);
    emptyAction.textContent = t("Connect {0}", disconnected.name);
    return;
  }
  emptyMode = "manual";
  emptyTitle.textContent = t("Your library is empty");
  emptyText.textContent = t("Add an executable, scan a folder, or connect a store.");
  emptyAction.textContent = t("＋ Add game");
}

function setLoading(value: boolean): void {
  gameGrid.setAttribute("aria-busy", String(value));
  skeleton.style.display = value && viewMode === "grid" && visibleGames.length === 0 ? "grid" : "none";
  if (value) {
    empty.hidden = true;
  }
}

function renderView(): void {
  const isDetails = viewMode === "details";
  sidebarToggle.hidden = !isDetails;
  detailsView.hidden = !isDetails;
  gridView.hidden = isDetails;
  for (const button of viewModeButtons) {
    const active = (button.dataset.viewMode || "details") === viewMode;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  }
  if (!isDetails) detailsTrailer.stop();
  if (!isDetails && gridDirty) {
    renderGrid();
  }
  if (isDetails) {
    renderDetailsList();
    if (detailsGame) {
      renderDetails();
    }
  }
}

const gridCards = new Map<string, PlaynuxGameCardElement>();

function renderGrid(): void {
  if (!gridDirty) return;
  const next = new Set(visibleGames.map((game) => game.id));
  for (const [id, card] of gridCards) {
    if (!next.has(id)) {
      card.remove();
      gridCards.delete(id);
    }
  }
  let anchor = gameGrid.firstElementChild;
  for (const game of visibleGames) {
    let card = gridCards.get(game.id);
    if (!card) {
      card = document.createElement("playnux-game-card") as PlaynuxGameCardElement;
      card.classList.toggle("is-selected", game.id === selectedGridId);
      gridCards.set(game.id, card);
    }
    if (card.game !== game) card.game = game;
    if (card !== anchor) gameGrid.insertBefore(card, anchor);
    anchor = card.nextElementSibling;
  }
  gameGrid.hidden = visibleGames.length === 0;
  gameCount.textContent = visibleGames.length === 1 ? t("1 game") : t("{0} games", visibleGames.length);
  updateEmpty(visibleGames);
  gridDirty = false;
}

let selectedGridId = "";
function selectGridCard(id: string): void {
  gridCards.get(selectedGridId)?.classList.remove("is-selected");
  selectedGridId = id;
  gridCards.get(selectedGridId)?.classList.add("is-selected");
}

function renderDetailsList(): void {
  if (viewMode !== "details") return;
  detailsCount.textContent = visibleGames.length === 1 ? "1" : String(visibleGames.length);
  if (visibleGames.length === 0) {
    const message = document.createElement("p");
    message.className = "details-list-empty";
    message.textContent = t("No games to display.");
    detailsList.replaceChildren(message);
    return;
  }

  const nodes: HTMLElement[] = [];
  for (const game of visibleGames) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "details-row";
    button.dataset.gameId = game.id;
    button.style.setProperty("--custom-contextmenu-data", game.id);
    button.classList.toggle("is-selected", detailsGame?.id === game.id);
    button.classList.toggle("is-uninstalled", !game.is_installed);
    button.setAttribute("aria-label", t("View {0}", game.name));

    const thumb = document.createElement("span");
    thumb.className = "details-row-thumb";
    const imageSource = remoteImage(game.icon);
    if (imageSource) {
      const image = document.createElement("img");
      image.alt = "";
      image.loading = "lazy";
      setArtworkImage(image, imageSource, "icon");
      image.addEventListener("error", () => {
        image.remove();
        thumb.textContent = initials(game.name);
      });
      thumb.append(image);
    } else {
      thumb.textContent = initials(game.name);
    }

    const copy = document.createElement("span");
    copy.className = "details-row-copy";
    const name = document.createElement("strong");
    name.textContent = game.name;
    const meta = document.createElement("span");
    meta.textContent = [sourceLabel(game), formatPlaytime(game.playtime)].filter(Boolean).join(" · ");
    copy.append(name, meta);

    const pip = document.createElement("span");
    pip.className = "details-row-pip";
    pip.hidden = !game.is_installed;
    pip.title = t("Installed");

    button.append(thumb, copy, pip);
    button.addEventListener("click", () => {
      void selectDetails(game);
    });
    nodes.push(button);
  }
  detailsList.replaceChildren(...nodes);
}

function ensureDetailsSelection(): void {
  const updated = visibleGames.find((game) => game.id === detailsGame?.id);
  if (updated) {
    detailsGame = updated;
    renderDetails();
    renderDetailsList();
    return;
  }
  detailsGame = visibleGames[0] || null;
  if (detailsGame) {
    void selectDetails(detailsGame);
  } else {
    renderDetails();
    renderDetailsList();
  }
}

function renderLibrary(): void {
  updateSectionTitle();
  updateCounts();
  gridDirty = true;
  if (viewMode === "grid") {
    renderGrid();
  }
  ensureDetailsSelection();
  const updatedDrawer = allGames.find((game) => game.id === selectedGame?.id);
  if (updatedDrawer) {
    selectedGame = updatedDrawer;
    renderDrawer(updatedDrawer);
  }
  renderView();
  if (bigPicture.isOpen()) {
    bigPicture.setGames(visibleGames);
  }
}

async function loadGames(filter: string, query: string): Promise<void> {
  const sequence = ++loadSequence;
  setLoading(true);
  try {
    const trimmed = query.trim();
    const result = trimmed
      ? await HostService.SearchGames(trimmed)
      : await HostService.GetGames(filter);
    if (sequence !== loadSequence) {
      return;
    }
    const games = cleanGames(result);
    if (!trimmed && filter === "all") {
      allGames = games;
      renderOrigins();
    }
    visibleGames = filterByOrigin(trimmed ? filterSearchResults(games, filter) : games);
    renderLibrary();
  } catch (error) {
    if (sequence === loadSequence) {
      visibleGames = [];
      renderLibrary();
      showToast(messageOf(error), true);
    }
  } finally {
    if (sequence === loadSequence) {
      setLoading(false);
    }
  }
}

async function reloadAll(notify: boolean, discoverNew = false): Promise<void> {
  const known = discoverNew ? new Set(allGames.map((game) => game.id)) : null;
  const sequence = ++loadSequence;
  setLoading(true);
  try {
    const [all, storeState] = await Promise.all([
      HostService.GetGames("all"),
      HostService.GetStores(),
    ]);
    if (sequence !== loadSequence) {
      return;
    }
    allGames = cleanGames(all);
    stores = storeState || [];
    integrations.refreshStores();
    renderOrigins();

    const query = searchQuery.trim();
    const result = query ? filterSearchResults(cleanGames(await HostService.SearchGames(query)), activeFilter)
      : activeFilter !== "all" ? cleanGames(await HostService.GetGames(activeFilter)) : allGames;
    if (sequence !== loadSequence) {
      return;
    }
    visibleGames = filterByOrigin(result);
    renderLibrary();
    if (known) {
      integrations.downloadNew(allGames.filter((game) => !known.has(game.id)).map((game) => game.id));
    }
    if (notify) {
      showToast(t("Library updated with {0} games.", allGames.length));
    }
  } catch (error) {
    if (sequence === loadSequence) {
      showToast(messageOf(error), true);
    }
  } finally {
    if (sequence === loadSequence) {
      setLoading(false);
    }
  }
}

function renderOrigins(): void {
  const selected = sourceFilter.value;
  const origins = new Map<string, string>();
  for (const game of allGames) {
    const id = game.plugin_id;
    if (id && !origins.has(id)) {
      origins.set(id, stores.find((store) => store.id === id)?.name || sourceLabel(game));
    }
  }
  const entries = Array.from(origins).sort((a, b) => a[1].localeCompare(b[1], language));
  sourceFilter.replaceChildren(new Option(t("All sources"), ""), ...entries.map(([id, label]) => new Option(label, id)));
  sourceFilter.value = origins.has(selected) ? selected : "";
}

function filterByOrigin(games: Game[]): Game[] {
  const id = sourceFilter.value;
  return id ? games.filter((game) => game.plugin_id === id) : games;
}

function filterSearchResults(games: Game[], filter: string): Game[] {
  switch (filter) {
    case "installed": return games.filter((game) => game.is_installed);
    case "favorites": return games.filter((game) => game.favorite);
    case "recent": return games.filter((game) => game.last_activity);
    default: return games;
  }
}

// ── Detalhes estilo Steam ────────────────────────────────────────────────────

async function selectDetails(game: Game, fetch = true): Promise<void> {
  detailsGame = game;
  renderDetails();
  renderDetailsList();
  if (!fetch || viewMode !== "details") {
    return;
  }
  try {
    const full = await HostService.GetGame(game.id);
    if (full && detailsGame?.id === game.id) {
      detailsGame = full;
      renderDetails();
      renderDetailsList();
    }
  } catch {
    // O resumo já contém o suficiente para o painel.
  }
}

function refreshBackgroundTrailers(): void {
  const enabled = uiSettings.background_trailer && !launching && !bigPicture.isOpen();
  const drawerOpen = drawer.classList.contains("is-open");
  detailsTrailer.set(detailsGame, enabled && viewMode === "details" && !drawerOpen);
  drawerTrailer.set(selectedGame, enabled && drawerOpen);
}

function renderDetails(): void {
  if (viewMode !== "details") return;
  const game = detailsGame;
  if (!game) {
    detailsHero.classList.add("is-empty");
    detailsPlaceholder.hidden = false;
    detailsBody.hidden = true;
    detailsBg.removeAttribute("src");
    detailsTrailer.stop();
    renderCompatibility(detailsCompatibility, null);
    return;
  }

  detailsHero.classList.remove("is-empty");
  detailsPlaceholder.hidden = true;
  detailsBody.hidden = false;

  const backdrop = game.background_image || game.cover_image || "";
  if (!backdrop) delete detailsCompatibility.dataset.backdrop;
  detailsBg.onload = () => sampleBackdrop(detailsBg, detailsCompatibility);
  setArtworkImage(detailsBg, backdrop, "banner");
  detailsBg.alt = "";
  detailsTitle.textContent = game.name;
  detailsLogo.onerror = () => {
    detailsLogo.hidden = true;
    detailsTitle.hidden = false;
  };
  if (game.logo_image) {
    setArtworkImage(detailsLogo, game.logo_image, "logo");
    detailsLogo.alt = game.name;
    detailsLogo.hidden = false;
    detailsTitle.hidden = true;
  } else {
    detailsLogo.removeAttribute("src");
    detailsLogo.hidden = true;
    detailsTitle.hidden = false;
  }

  const launchable = canLaunch(game);
  detailsPlay.disabled = launching;
  if (!game.is_installed) {
    detailsPlay.textContent = t("INSTALL");
    detailsPlay.title = t("Install {0} through its client or add an executable", game.name);
    detailsPlay.classList.add("is-install");
    detailsPlay.classList.remove("is-disabled");
  } else {
    detailsPlay.textContent = launching ? t("STARTING…") : launchable ? t("▶ PLAY") : t("UNAVAILABLE");
    detailsPlay.title = launchable ? t("Play {0}", game.name) : launchBlockReason(game);
    detailsPlay.classList.remove("is-install");
    detailsPlay.classList.toggle("is-disabled", !launchable);
  }

  renderGameInformation(game, detailsDescription, detailsFacts, element("details-tags"));

  refreshBackgroundTrailers();

  renderCompatibility(detailsCompatibility, game);

  // Restart the subtle hero fade when switching games without rebuilding the
  // whole panel (keeps scrolling and focus stable).
  detailsBody.classList.remove("is-entering");
  void detailsBody.offsetWidth;
  detailsBody.classList.add("is-entering");
}





// ── Drawer (grade) ───────────────────────────────────────────────────────────

function setDrawerPlayButton(game: Game): void {
  const launchable = canLaunch(game);
  drawerPlay.disabled = launching;
  if (!game.is_installed) {
    drawerPlay.textContent = t("Install");
    drawerPlay.title = t("Install {0}", game.name);
    drawerPlay.classList.add("secondary");
    return;
  }
  drawerPlay.classList.toggle("secondary", !launchable);
  if (!launchable) {
    drawerPlay.textContent = t("Unavailable");
    drawerPlay.title = launchBlockReason(game);
    return;
  }
  drawerPlay.textContent = launching ? t("Launching…") : t("▶ Play");
  drawerPlay.title = t("Play {0}", game.name);
}

function renderDrawer(game: Game): void {
  drawerSource.textContent = sourceLabel(game);
  drawerTitle.textContent = game.name;
  drawerInitial.textContent = initials(game.name);

  drawerCover.alt = "";
  drawerCover.hidden = true;
  drawerCover.onload = () => {
    drawerCover.hidden = false;
    sampleBackdrop(drawerCover, drawerCompatibility);
  };
  drawerCover.onerror = () => {
    drawerCover.hidden = true;
  };
  const backdrop = game.background_image || game.cover_image;
  if (!backdrop) delete drawerCompatibility.dataset.backdrop;
  setArtworkImage(drawerCover, backdrop, "banner");
  if (drawerCover.complete && drawerCover.naturalWidth) drawerCover.hidden = false;

  const size = formatSize(game.install_size);

  if (!canLaunch(game)) {
    drawerInstalled.textContent = game.plugin_id === "xbox" ? t("Library only (Xbox)") : t("Emulator not found");
  } else if (game.is_installed) {
    drawerInstalled.textContent = size ? t("Installed · {0}", size) : t("Installed");
  } else {
    drawerInstalled.textContent = t("Not installed");
  }

  renderGameInformation(game, drawerDescription, drawerFacts, element("drawer-tags"));

  refreshBackgroundTrailers();
  setDrawerPlayButton(game);
  renderCompatibility(drawerCompatibility, game);
}

async function openDrawer(game: Game): Promise<void> {
  selectGridCard(game.id);
  lastFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  selectedGame = game;
  drawer.classList.add("is-open");
  drawer.setAttribute("aria-hidden", "false");
  renderDrawer(game);
  try {
    const full = await HostService.GetGame(game.id);
    if (full && selectedGame?.id === game.id) {
      selectedGame = full;
      renderDrawer(full);
    }
  } catch {
    // O resumo já permite abrir o drawer.
  }
  if (!drawer.classList.contains("is-open")) {
    return;
  }
  if (drawerPlay.disabled && drawerClose) {
    drawerClose.focus({ preventScroll: true });
  } else {
    drawerPlay.focus({ preventScroll: true });
  }
}

function closeDrawer(): void {
  if (!drawer.classList.contains("is-open")) {
    return;
  }
  drawer.classList.remove("is-open");
  drawer.setAttribute("aria-hidden", "true");
  selectedGame = null;
  refreshBackgroundTrailers();
  if (lastFocused && lastFocused.isConnected) {
    lastFocused.focus({ preventScroll: true });
  }
  lastFocused = null;
}

async function launchGame(game: Game, origin: "details" | "drawer" | "big" = "details"): Promise<void> {
  if (launching) {
    return;
  }
  if (!canLaunch(game)) {
    const message = launchBlockReason(game);
    recordGameError(game.name, t("Launch"), message);
    showToast(message, true);
    return;
  }

  launching = true;
  refreshBackgroundTrailers();
  if (detailsGame?.id === game.id) {
    renderDetails();
  }
  if (selectedGame?.id === game.id) {
    setDrawerPlayButton(game);
  }
  try {
    await HostService.LaunchGame(game.id);
    showToast(t("Launching {0}… Deep Sleep begins once the game opens.", game.name));
    if (origin === "drawer") {
      closeDrawer();
    }
    if (origin === "big") {
      bigPicture.close();
    }
  } catch (error) {
    const message = messageOf(error);
    recordGameError(game.name, t("Launch"), message);
    showToast(message, true);
  } finally {
    launching = false;
    refreshBackgroundTrailers();
    if (detailsGame?.id === game.id) {
      renderDetails();
    }
    if (selectedGame?.id === game.id) {
      setDrawerPlayButton(selectedGame);
    }
  }
}

// ── Manual / pastas ──────────────────────────────────────────────────────────

function setManualBusy(value: boolean): void {
  manualBusy = value;
  manualSubmit.disabled = value;
  manualPick.disabled = value;
  manualPath.disabled = value;
  manualName.disabled = value;
}

function manualErrorText(message: string): void {
  manualError.textContent = message;
  manualError.hidden = !message;
}

function suggestName(path: string): string {
  const base = path.split("/").filter(Boolean).pop() || path;
  return base.replace(/\.(sh|x86_64|x86|AppImage|exe)$/i, "").replace(/[_.]+/g, " ").trim();
}

function renderApps(): void {
  const query = appsSearch.value.trim().toLowerCase();
  const visible = localApps.filter((app) => !query || app.name.toLowerCase().includes(query));
  if (!visible.length) {
    const empty = document.createElement("p");
    empty.className = "apps-empty";
    empty.textContent = appsLoading ? t("Finding apps…") : t("No game apps found.");
    appsList.replaceChildren(empty);
    appsAdd.disabled = true;
    return;
  }
  const rows: HTMLElement[] = [];
  for (const app of visible) {
    const row = document.createElement("label");
    row.className = "app-row";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = selectedApps.has(app.id);
    checkbox.disabled = app.added;
    checkbox.addEventListener("change", () => {
      if (checkbox.checked) selectedApps.add(app.id);
      else selectedApps.delete(app.id);
      appsAdd.disabled = selectedApps.size === 0;
    });
    const icon = document.createElement("span");
    icon.className = "app-icon";
    if (remoteImage(app.icon)) {
      const image = document.createElement("img");
      image.src = app.icon;
      image.alt = "";
      image.loading = "lazy";
      image.addEventListener("error", () => image.remove());
      icon.append(image);
    } else {
      icon.textContent = initials(app.name);
    }
    const copy = document.createElement("span");
    copy.className = "app-copy";
    const name = document.createElement("strong");
    name.textContent = app.name;
    const meta = document.createElement("span");
    meta.textContent = app.added ? t("{0} · Already in the library", app.source) : app.source;
    copy.append(name, meta);
    row.append(checkbox, icon, copy);
    rows.push(row);
  }
  appsList.replaceChildren(...rows);
  appsAdd.disabled = selectedApps.size === 0;
}

async function loadLocalApps(): Promise<void> {
  if (appsLoading) return;
  appsLoading = true;
  appsAdd.disabled = true;
  renderApps();
  try {
    localApps = (await HostService.LocalApps()) ?? [];
    for (const app of localApps) {
      if (app.added) selectedApps.delete(app.id);
    }
    appsRefresh.disabled = false;
  } catch (error) {
    localApps = [];
    manualErrorText(messageOf(error));
  } finally {
    appsLoading = false;
    renderApps();
  }
}

async function addSelectedApps(): Promise<void> {
  const ids = [...selectedApps];
  if (!ids.length || manualBusy) return;
  manualBusy = true;
  appsAdd.disabled = true;
  manualErrorText("");
  try {
    await HostService.AddLocalApps(ids);
    selectedApps.clear();
    await reloadAll(false, true);
    await loadLocalApps();
    showToast(t("{0} apps added to the library.", ids.length));
  } catch (error) {
    manualErrorText(messageOf(error));
  } finally {
    manualBusy = false;
    appsAdd.disabled = selectedApps.size === 0;
  }
}

function openManualDialog(): void {
  manualForm.reset();
  manualErrorText("");
  selectedApps = new Set();
  appsSearch.value = "";
  renderApps();
  if (!manualDialog.open) {
    manualDialog.showModal();
  }
  void loadLocalApps();
  manualPath.focus();
}

async function pickManualExecutable(): Promise<void> {
  try {
    const path = await HostService.PickGameExecutable();
    if (path) {
      manualPath.value = path;
      if (!manualName.value.trim()) {
        manualName.value = suggestName(path);
      }
    }
  } catch (error) {
    const message = messageOf(error);
    if (!/cancel/i.test(message)) {
      manualErrorText(message);
    }
  }
}

async function addManualGame(): Promise<void> {
  if (manualBusy) {
    return;
  }
  const path = manualPath.value.trim();
  if (!path) {
    manualErrorText(t("Choose or enter an executable path."));
    manualPath.focus();
    return;
  }
  setManualBusy(true);
  manualErrorText("");
  try {
    const game = await HostService.AddManualGame(path, manualName.value.trim());
    if (game) {
      manualDialog.close();
      await reloadAll(false, true);
      await selectDetails(game);
      showToast(t("{0} added to the library.", game.name));
    }
  } catch (error) {
    manualErrorText(messageOf(error));
  } finally {
    setManualBusy(false);
  }
}

// ── Eventos ─────────────────────────────────────────────────────────────────

function setViewMode(mode: ViewMode, persist = true): void {
  viewMode = mode;
  localStorage.setItem("playnux:view", mode);
  renderView();
  if (persist) {
    uiSettings = { ...uiSettings, view_mode: mode };
    void HostService.SaveUISettings(uiSettings).catch(() => undefined);
  }
}

function openBigPicture(): void {
  if (visibleGames.length === 0) {
    showToast(t("There are no games in this view to open in Big Picture."), true);
    return;
  }
  void bigPicture.open(visibleGames, detailsGame?.id || "").then(refreshBackgroundTrailers).catch((error) => {
    showToast(t("Could not enter full screen. {0}", messageOf(error)), true);
  });
}

function setupEventListeners(): void {
  configureDetailsSidebar(detailsView, sidebarToggle);
  configureGridCards(gameGrid);
  document.querySelectorAll<HTMLElement>("[data-close-drawer]").forEach((control) => {
    control.addEventListener("click", closeDrawer);
  });

  detailsPlay.addEventListener("click", () => {
    if (detailsGame) {
      void launchGame(detailsGame, "details");
    }
  });

  drawerPlay.addEventListener("click", () => {
    if (selectedGame) {
      void launchGame(selectedGame, "drawer");
    }
  });
  document.addEventListener("visibilitychange", refreshBackgroundTrailers);
  document.addEventListener("playnux:media-visibility", refreshBackgroundTrailers);

  bigPictureButton.addEventListener("click", openBigPicture);

  for (const button of viewModeButtons) {
    button.addEventListener("click", () => {
      setViewMode(button.dataset.viewMode === "grid" ? "grid" : "details");
    });
  }

  const applyZoom = (): void => {
    document.documentElement.style.setProperty("--card-min", `${gridZoom.value}px`);
    localStorage.setItem("playnux:zoom", gridZoom.value);
  };
  const storedZoom = localStorage.getItem("playnux:zoom");
  if (storedZoom) {
    gridZoom.value = storedZoom;
  }
  applyZoom();
  gridZoom.addEventListener("input", applyZoom);

  // System Menu Dropdown
  const systemMenuTrigger = element<HTMLButtonElement>("system-menu-trigger");
  const systemMenuPopover = element<HTMLElement>("system-menu-popover");

  const toggleSystemMenu = (open?: boolean) => {
    const isExpanded = systemMenuTrigger.getAttribute("aria-expanded") === "true";
    const next = typeof open === "boolean" ? open : !isExpanded;
    systemMenuTrigger.setAttribute("aria-expanded", String(next));
    systemMenuPopover.hidden = !next;
  };

  systemMenuTrigger.addEventListener("click", (e) => {
    e.stopPropagation();
    toggleSystemMenu();
  });

  document.addEventListener("click", (e) => {
    if (!systemMenuTrigger.contains(e.target as Node) && !systemMenuPopover.contains(e.target as Node)) {
      toggleSystemMenu(false);
    }
  });

  element("menu-add-game").addEventListener("click", () => {
    toggleSystemMenu(false);
    openManualDialog();
  });
  element("menu-scan-local").addEventListener("click", () => {
    toggleSystemMenu(false);
    integrations.open("stores", "linux");
  });
  element("menu-scan-roms").addEventListener("click", () => {
    toggleSystemMenu(false);
    integrations.open("stores", "emulation");
  });
  element("menu-metadata").addEventListener("click", () => {
    toggleSystemMenu(false);
    integrations.openMetadata();
  });
  element("menu-plugins").addEventListener("click", () => {
    toggleSystemMenu(false);
    integrations.open("stores");
  });
  element("menu-settings").addEventListener("click", () => {
    toggleSystemMenu(false);
    integrations.open("appearance");
  });
  element("menu-big-picture").addEventListener("click", () => {
    toggleSystemMenu(false);
    openBigPicture();
  });
  manualCancel.addEventListener("click", () => manualDialog.close());
  manualPick.addEventListener("click", () => void pickManualExecutable());
  appsRefresh.addEventListener("click", () => {
    appsRefresh.disabled = true;
    void loadLocalApps();
  });
  appsSearch.addEventListener("input", renderApps);
  appsAdd.addEventListener("click", () => void addSelectedApps());
  manualForm.addEventListener("submit", (event) => {
    event.preventDefault();
    void addManualGame();
  });
  manualScanFolder.addEventListener("click", () => {
    manualDialog.close();
    integrations.open("stores", "linux");
  });
  manualDialog.addEventListener("close", () => {
    setManualBusy(false);
    manualErrorText("");
  });


  emptyAction.addEventListener("click", () => {
    switch (emptyMode) {
      case "clear":
        searchInput.value = "";
        searchQuery = "";
        updateSectionTitle();
        void loadGames(activeFilter, "");
        break;
      case "all":
        activeFilter = "all";
        sourceFilter.value = "";
        updateFilterButtons();
        updateSectionTitle();
        void loadGames("all", "");
        break;
      case "login": {
        const store = stores.find((item) => item.id === emptyStoreID);
        if (store) {
          integrations.open("stores", store.id);
        }
        break;
      }
      case "manual":
        openManualDialog();
        break;
      default:
        void reloadAll(true);
    }
  });

  searchInput.addEventListener("input", () => {
    searchQuery = searchInput.value;
    updateSectionTitle();
    window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(() => {
      void loadGames(activeFilter, searchQuery);
    }, 180);
  });

  for (const button of filterButtons) {
    button.addEventListener("click", () => {
      activeFilter = button.dataset.filter || "all";
      searchQuery = "";
      searchInput.value = "";
      window.clearTimeout(searchTimer);
      updateFilterButtons();
      updateSectionTitle();
      void loadGames(activeFilter, "");
    });
  }

  sourceFilter.addEventListener("change", () => {
    window.clearTimeout(searchTimer);
    void loadGames(activeFilter, searchQuery);
  });

  gameGrid.addEventListener("playnux:open", (event) => {
    const game = (event as CustomEvent<Game>).detail;
    if (game) {
      void openDrawer(game);
    }
  });

  gameGrid.addEventListener("playnux:play", (event) => {
    const game = (event as CustomEvent<Game>).detail;
    if (game) {
      void launchGame(game, "drawer");
    }
  });

  document.addEventListener("keydown", (event) => {
    if (bigPicture.isOpen()) {
      return;
    }
    if (event.key === "Escape") {
      if (drawer.classList.contains("is-open")) {
        closeDrawer();
        return;
      }
      if (searchQuery) {
        searchInput.value = "";
        searchQuery = "";
        updateSectionTitle();
        void loadGames(activeFilter, "");
      }
      return;
    }

    if (event.key === "/") {
      const target = event.target;
      const typing =
        target instanceof HTMLElement &&
        (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable);
      if (!typing) {
        event.preventDefault();
        searchInput.focus();
      }
    }
  });
}

function setupSkeleton(): void {
  const cards: HTMLElement[] = [];
  for (let index = 0; index < 12; index++) {
    const card = document.createElement("div");
    card.className = "skeleton-card";
    cards.push(card);
  }
  skeleton.replaceChildren(...cards);
}

installDeepSleep();
setupSkeleton();
setupEventListeners();
updateFilterButtons();
updateSectionTitle();
updateCounts();
empty.hidden = true;
renderView();
function applyDownloadedGames(games: Game[]): void {
  const byID = new Map(games.map((game) => [game.id, game]));
  allGames = allGames.map((game) => byID.get(game.id) ?? game);
  visibleGames = visibleGames.map((game) => byID.get(game.id) ?? game);
  renderLibrary();
}

const integrations = setupIntegrations({
  stores: () => stores,
  games: () => allGames,
  filteredGames: () => visibleGames,
  selected: () => selectedGame || detailsGame,
  addGame: openManualDialog,
  reload: (discoverNew = false) => reloadAll(false, discoverNew),
  applyDownloads: applyDownloadedGames,
  recordGameError,
  error: messageOf,
  report: showToast,
  applyAppearance: (settings) => {
    uiSettings = applyUISettings(settings);
    refreshBackgroundTrailers();
  },
  setViewMode: (mode) => setViewMode(mode, false),
});

function applyUpdatedGame(updated: Game): void {
  allGames = allGames.map((item) => item.id === updated.id ? updated : item);
  visibleGames = visibleGames.map((item) => item.id === updated.id ? updated : item);
  if (detailsGame?.id === updated.id) {
    detailsGame = updated;
    renderDetails();
  }
  if (selectedGame?.id === updated.id) {
    selectedGame = updated;
    renderDrawer(updated);
  }
  renderLibrary();
}

const properties = setupGameProperties({
  error: messageOf,
  recordGameError,
  toast: showToast,
  onGameUpdated: applyUpdatedGame,
});

element("details-launch-options").addEventListener("click", () => {
  if (detailsGame) properties.open(detailsGame, "general");
});
element("drawer-launch-options").addEventListener("click", () => {
  if (selectedGame) properties.open(selectedGame, "general");
});

Events.On("gameplay:launch-error", (event) => {
  const result = event.data as { game?: string; message?: string } | null;
  if (!result?.game || !result.message) return;
  recordGameError(result.game, t("Game process"), result.message);
  showToast(t("{0} failed. See Notifications.", result.game), true);
});

Events.On("library:game-menu", (event) => {
  const data = event.data as { id?: string; action?: string } | undefined;
  const game = visibleGames.find((item) => item.id === data?.id);
  if (!game) return;
  switch (data?.action) {
    case "play": void launchGame(game); break;
    case "details":
      if (viewMode === "details") void selectDetails(game);
      else void openDrawer(game);
      break;
    case "metadata": properties.open(game, "general"); break;
    case "cover": properties.open(game, "artwork"); break;
    case "icon": properties.open(game, "artwork"); break;
    case "launch": properties.open(game, "launch"); break;
  }
});

async function start(): Promise<void> {
  void HostService.SetInterfaceLanguage(language).catch(() => undefined);
  // Local scanning is manual; startup only restores saved appearance and the
  // already imported library.
  try {
    uiSettings = applyUISettings(await HostService.GetUISettings());
    if (uiSettings.view_mode === "grid" || uiSettings.view_mode === "details") {
      setViewMode(uiSettings.view_mode, false);
    }
  } catch {
    applyUISettings(uiSettings);
  }
  await reloadAll(false);
}

void start();
const openUpdates = setupUpdates(showToast);
element("menu-updates").addEventListener("click", () => {
  const trigger = element("system-menu-trigger");
  if (trigger.getAttribute("aria-expanded") === "true") trigger.click();
  openUpdates();
});
