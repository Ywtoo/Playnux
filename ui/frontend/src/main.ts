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
import { trailerEmbedURL } from "./media";

const FILTER_TITLES: Record<string, string> = {
  all: "Todos os jogos",
  installed: "Instalados",
  favorites: "Favoritos",
  recent: "Recentes",
};

type ViewMode = "details" | "grid";
type EmptyMode = "refresh" | "clear" | "all" | "login" | "manual";

function element<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (!found) {
    throw new Error(`elemento #${id} não encontrado`);
  }
  return found as T;
}

const searchInput = element<HTMLInputElement>("search");
const refreshButton = element<HTMLButtonElement>("refresh");
const addGameButton = element<HTMLButtonElement>("add-game");
const bigPictureButton = element<HTMLButtonElement>("big-picture-open");
const viewModeButtons = Array.from(document.querySelectorAll<HTMLButtonElement>(".view-mode"));

const detailsView = element<HTMLElement>("details-view");
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
const detailsTrailer = element<HTMLButtonElement>("details-trailer");
const detailsTrailerPlayer = element<HTMLElement>("details-trailer-player");
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
const drawerTrailer = element<HTMLElement>("drawer-trailer");
const drawerTrailerPlay = element<HTMLButtonElement>("drawer-trailer-play");
const drawerTrailerPlayer = element<HTMLElement>("drawer-trailer-player");
const drawerCompatibility = element<HTMLElement>("drawer-compatibility");

const addLocalFolder = element<HTMLButtonElement>("add-local-folder");
const addRomFolder = element<HTMLButtonElement>("add-rom-folder");
const folderDialog = element<HTMLDialogElement>("folder-dialog");
const folderForm = element<HTMLFormElement>("folder-form");
const folderTitle = element<HTMLElement>("folder-title");
const folderHint = element<HTMLElement>("folder-hint");
const folderPath = element<HTMLInputElement>("folder-path");
const folderList = element<HTMLElement>("folder-list");
const folderCancel = element<HTMLButtonElement>("folder-cancel");
const folderSubmit = element<HTMLButtonElement>("folder-submit");

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
const filterButtons = Array.from(document.querySelectorAll<HTMLButtonElement>(".filter"));
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
let folderKind: "local" | "rom" = "local";
let folderBusy = false;
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
  onError: (error) => showToast(`Não foi possível restaurar a janela. ${messageOf(error)}`, true),
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
  return cleaned || "Não foi possível concluir a operação.";
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
  const title = searchQuery.trim() ? "Resultados da busca" : FILTER_TITLES[activeFilter] || "Biblioteca";
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
    emptyTitle.textContent = "Nada encontrado";
    emptyText.textContent = `Nenhum jogo corresponde a “${searchQuery.trim()}”. Tente outro termo.`;
    emptyAction.textContent = "Limpar busca";
    return;
  }
  if (activeFilter !== "all" || sourceFilter.value) {
    emptyMode = "all";
    emptyTitle.textContent = "Nenhum jogo nesta categoria";
    emptyText.textContent = "Sua biblioteca não tem jogos com esse filtro. Veja a biblioteca completa ou adicione um jogo.";
    emptyAction.textContent = "Ver todos os jogos";
    return;
  }
  const disconnected = stores.find((store) => !store.connected && store.id !== "xbox");
  if (disconnected && allGames.length === 0) {
    emptyMode = "login";
    emptyStoreID = disconnected.id;
    emptyTitle.textContent = "Conecte sua primeira loja";
    emptyText.textContent = `Entre na ${disconnected.name} ou importe os jogos instalados sem senha.`;
    emptyAction.textContent = `Conectar ${disconnected.name}`;
    return;
  }
  emptyMode = "manual";
  emptyTitle.textContent = "Sua biblioteca está vazia";
  emptyText.textContent = "Adicione um executável, escaneie uma pasta ou conecte uma loja.";
  emptyAction.textContent = "＋ Adicionar jogo";
}

function setLoading(value: boolean): void {
  gameGrid.setAttribute("aria-busy", String(value));
  refreshButton.disabled = value;
  skeleton.style.display = value && viewMode === "grid" && visibleGames.length === 0 ? "grid" : "none";
  if (value) {
    empty.hidden = true;
  }
}

function renderView(): void {
  const isDetails = viewMode === "details";
  detailsView.hidden = !isDetails;
  gridView.hidden = isDetails;
  for (const button of viewModeButtons) {
    const active = (button.dataset.viewMode || "details") === viewMode;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", String(active));
  }
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

function renderGrid(): void {
  const cards: PlaynuxGameCardElement[] = [];
  for (const game of visibleGames) {
    const card = document.createElement("playnux-game-card") as unknown as PlaynuxGameCardElement;
    card.game = game;
    cards.push(card);
  }
  gameGrid.replaceChildren(...cards);
  gameGrid.hidden = visibleGames.length === 0;
  gameCount.textContent = visibleGames.length === 1 ? "1 jogo" : `${visibleGames.length} jogos`;
  updateEmpty(visibleGames);
  gridDirty = false;
}

function renderDetailsList(): void {
  detailsCount.textContent = visibleGames.length === 1 ? "1" : String(visibleGames.length);
  if (visibleGames.length === 0) {
    const message = document.createElement("p");
    message.className = "details-list-empty";
    message.textContent = "Nenhum jogo para mostrar.";
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
    button.setAttribute("aria-label", `Ver ${game.name}`);

    const thumb = document.createElement("span");
    thumb.className = "details-row-thumb";
    const imageSource = remoteImage(game.icon);
    if (imageSource) {
      const image = document.createElement("img");
      image.src = imageSource;
      image.alt = "";
      image.loading = "lazy";
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
    pip.title = "Instalado";

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
      showToast(`Biblioteca atualizada: ${allGames.length} jogos.`);
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
  const entries = Array.from(origins).sort((a, b) => a[1].localeCompare(b[1], "pt-BR"));
  sourceFilter.replaceChildren(new Option("Todas as origens", ""), ...entries.map(([id, label]) => new Option(label, id)));
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
  if (!fetch) {
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

function resetDetailsTrailer(): void {
  detailsTrailerPlayer.replaceChildren();
  detailsTrailerPlayer.hidden = true;
}

function renderDetails(): void {
  const game = detailsGame;
  if (!game) {
    detailsHero.classList.add("is-empty");
    detailsPlaceholder.hidden = false;
    detailsBody.hidden = true;
    detailsBg.removeAttribute("src");
    resetDetailsTrailer();
    renderCompatibility(detailsCompatibility, null, showToast);
    return;
  }

  detailsHero.classList.remove("is-empty");
  detailsPlaceholder.hidden = true;
  detailsBody.hidden = false;

  const backdrop = game.background_image || game.cover_image || "";
  if (backdrop) {
    detailsBg.src = backdrop;
  } else {
    detailsBg.removeAttribute("src");
  }
  detailsBg.alt = "";
  detailsTitle.textContent = game.name;
  detailsLogo.onerror = () => {
    detailsLogo.hidden = true;
    detailsTitle.hidden = false;
  };
  if (game.logo_image) {
    detailsLogo.src = game.logo_image;
    detailsLogo.alt = game.name;
    detailsLogo.hidden = false;
    detailsTitle.hidden = true;
  } else {
    detailsLogo.removeAttribute("src");
    detailsLogo.hidden = true;
    detailsTitle.hidden = false;
  }

  const launchable = canLaunch(game);
  detailsPlay.disabled = !launchable || launching;
  detailsPlay.textContent = launching ? "INICIANDO…" : launchable ? "▶ JOGAR" : "INDISPONÍVEL";
  detailsPlay.title = launchable ? `Jogar ${game.name}` : launchBlockReason(game);
  detailsPlay.classList.toggle("is-disabled", !launchable);

  renderGameInformation(game, detailsDescription, detailsFacts, element("details-tags"));

  resetDetailsTrailer();
  detailsTrailer.hidden = !game.trailer_url;

  renderCompatibility(detailsCompatibility, game, showToast);

  // Restart the subtle hero fade when switching games without rebuilding the
  // whole panel (keeps scrolling and focus stable).
  detailsBody.classList.remove("is-entering");
  void detailsBody.offsetWidth;
  detailsBody.classList.add("is-entering");
}

function playDetailsTrailer(): void {
  if (!detailsGame?.trailer_url) {
    return;
  }
  const url = trailerEmbedURL(detailsGame.trailer_url);
  if (!url) {
    window.open(detailsGame.trailer_url, "_blank", "noopener");
    return;
  }
  const frame = document.createElement("iframe");
  frame.src = url;
  frame.title = `Trailer de ${detailsGame.name}`;
  frame.allow = "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture";
  frame.allowFullscreen = true;
  frame.referrerPolicy = "strict-origin-when-cross-origin";
  detailsTrailerPlayer.replaceChildren(frame);
  detailsTrailerPlayer.hidden = false;
  detailsTrailerPlayer.scrollIntoView({ block: "nearest" });
}



// ── Drawer (grade) ───────────────────────────────────────────────────────────

function setDrawerPlayButton(game: Game): void {
  const launchable = canLaunch(game);
  drawerPlay.classList.toggle("secondary", !launchable);
  drawerPlay.disabled = !launchable || launching;
  if (!launchable) {
    drawerPlay.textContent = "Indisponível";
    drawerPlay.title = launchBlockReason(game);
    return;
  }
  drawerPlay.textContent = launching ? "Iniciando…" : "▶ Jogar";
  drawerPlay.title = `Jogar ${game.name}`;
}

function renderDrawer(game: Game): void {
  drawerSource.textContent = sourceLabel(game);
  drawerTitle.textContent = game.name;
  drawerInitial.textContent = initials(game.name);

  drawerCover.alt = "";
  drawerCover.hidden = true;
  drawerCover.onload = () => {
    drawerCover.hidden = false;
  };
  drawerCover.onerror = () => {
    drawerCover.hidden = true;
  };
  if (game.cover_image) {
    drawerCover.src = game.cover_image;
  } else {
    drawerCover.removeAttribute("src");
    drawerCover.hidden = true;
  }

  const size = formatSize(game.install_size);

  if (!canLaunch(game)) {
    drawerInstalled.textContent = game.plugin_id === "xbox" ? "Somente biblioteca (Xbox)" : "Emulador não encontrado";
  } else if (game.is_installed) {
    drawerInstalled.textContent = size ? `Instalado · ${size}` : "Instalado";
  } else {
    drawerInstalled.textContent = "Não instalado";
  }

  renderGameInformation(game, drawerDescription, drawerFacts, element("drawer-tags"));

  resetTrailer(game.trailer_url);
  setDrawerPlayButton(game);
  renderCompatibility(drawerCompatibility, game, showToast);
}

function resetTrailer(url: string | null | undefined): void {
  drawerTrailerPlayer.replaceChildren();
  drawerTrailerPlayer.hidden = true;
  drawerTrailerPlay.hidden = false;
  const hasTrailer = Boolean(url && url.trim());
  drawerTrailer.hidden = !hasTrailer;
  drawerTrailerPlay.title = hasTrailer ? "Reproduzir trailer oficial" : "";
}

function playTrailer(): void {
  if (!selectedGame?.trailer_url) {
    return;
  }
  const url = trailerEmbedURL(selectedGame.trailer_url);
  if (!url) {
    window.open(selectedGame.trailer_url, "_blank", "noopener");
    return;
  }
  const frame = document.createElement("iframe");
  frame.src = url;
  frame.title = `Trailer de ${selectedGame.name}`;
  frame.allow = "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture";
  frame.allowFullscreen = true;
  frame.referrerPolicy = "strict-origin-when-cross-origin";
  drawerTrailerPlayer.replaceChildren(frame);
  drawerTrailerPlayer.hidden = false;
  drawerTrailerPlay.hidden = true;
}

async function openDrawer(game: Game): Promise<void> {
  lastFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  selectedGame = game;
  renderDrawer(game);
  drawer.classList.add("is-open");
  drawer.setAttribute("aria-hidden", "false");
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
  resetTrailer("");
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
    showToast(launchBlockReason(game), true);
    return;
  }

  launching = true;
  if (detailsGame?.id === game.id) {
    renderDetails();
  }
  if (selectedGame?.id === game.id) {
    setDrawerPlayButton(game);
  }
  try {
    await HostService.LaunchGame(game.id);
    showToast(`Iniciando ${game.name}… O Deep Sleep será ativado quando o jogo abrir.`);
    if (origin === "drawer") {
      closeDrawer();
    }
    if (origin === "big") {
      bigPicture.close();
    }
  } catch (error) {
    showToast(messageOf(error), true);
  } finally {
    launching = false;
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
    empty.textContent = appsLoading ? "Procurando aplicativos…" : "Nenhum app de jogo encontrado.";
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
    meta.textContent = app.added ? `${app.source} · já na biblioteca` : app.source;
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
    showToast(`${ids.length} app(s) adicionado(s) à biblioteca.`);
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
    manualErrorText("Selecione ou digite o caminho de um executável.");
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
      showToast(`${game.name} adicionado à biblioteca.`);
    }
  } catch (error) {
    manualErrorText(messageOf(error));
  } finally {
    setManualBusy(false);
  }
}

function setFolderBusy(value: boolean): void {
  folderBusy = value;
  folderSubmit.disabled = value;
  folderPath.disabled = value;
}

function renderFolderList(folders: string[]): void {
  if (folders.length === 0) {
    const emptyText = document.createElement("p");
    emptyText.className = "folder-empty";
    emptyText.textContent = "Nenhuma pasta adicionada ainda.";
    folderList.replaceChildren(emptyText);
    return;
  }
  const rows = folders.map((folder) => {
    const row = document.createElement("div");
    row.className = "folder-row";
    const path = document.createElement("span");
    path.textContent = folder;
    path.title = folder;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "folder-remove";
    remove.textContent = "Remover";
    remove.addEventListener("click", () => void removeFolder(folder));
    row.append(path, remove);
    return row;
  });
  folderList.replaceChildren(...rows);
}

async function refreshFolderList(): Promise<void> {
  const folders = folderKind === "local"
    ? await HostService.GetLocalFolders()
    : await HostService.GetRomFolders();
  renderFolderList(folders || []);
}

async function openFolderDialog(kind: "local" | "rom"): Promise<void> {
  folderKind = kind;
  const isLocal = kind === "local";
  folderTitle.textContent = isLocal ? "Escanear pasta de jogos" : "Adicionar pasta de ROMs";
  folderHint.textContent = isLocal
    ? "O Playnux procura ELF com permissão +x e scripts .sh. Cada subpasta vira um jogo."
    : "A pasta deve seguir o padrão EmuDeck/EmulationStation (ps2, switch, snes…). As pastas padrão de ~/ROMs já são verificadas.";
  folderPath.placeholder = isLocal ? "~/Games" : "~/ROMs/ps2";
  folderPath.value = "";
  renderFolderList([]);
  if (!folderDialog.open) {
    folderDialog.showModal();
  }
  folderPath.focus();
  try {
    await refreshFolderList();
  } catch (error) {
    showToast(messageOf(error), true);
  }
}

async function addFolder(): Promise<void> {
  if (folderBusy) {
    return;
  }
  const path = folderPath.value.trim();
  if (!path) {
    folderPath.focus();
    return;
  }

  setFolderBusy(true);
  try {
    const games = folderKind === "local"
      ? await HostService.AddLocalFolder(path)
      : await HostService.AddRomFolder(path);
    showToast(`${cleanGames(games).length} jogos encontrados na fonte local.`);
    folderDialog.close();
    await reloadAll(false, true);
  } catch (error) {
    showToast(messageOf(error), true);
  } finally {
    setFolderBusy(false);
  }
}

async function removeFolder(path: string): Promise<void> {
  if (folderBusy) {
    return;
  }
  setFolderBusy(true);
  try {
    const games = folderKind === "local"
      ? await HostService.RemoveLocalFolder(path)
      : await HostService.RemoveRomFolder(path);
    await refreshFolderList();
    await reloadAll(false);
    showToast(`Pasta removida. ${cleanGames(games).length} jogos permanecem na fonte local.`);
  } catch (error) {
    showToast(messageOf(error), true);
  } finally {
    setFolderBusy(false);
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
    showToast("Não há jogos na visualização atual para abrir no Big Picture.", true);
    return;
  }
  void bigPicture.open(visibleGames, detailsGame?.id || "").catch((error) => {
    showToast(`Não foi possível abrir a tela cheia. ${messageOf(error)}`, true);
  });
}

function setupEventListeners(): void {
  document.querySelectorAll<HTMLElement>("[data-close-drawer]").forEach((control) => {
    control.addEventListener("click", closeDrawer);
  });

  detailsPlay.addEventListener("click", () => {
    if (detailsGame) {
      void launchGame(detailsGame, "details");
    }
  });
  detailsTrailer.addEventListener("click", playDetailsTrailer);

  drawerPlay.addEventListener("click", () => {
    if (selectedGame) {
      void launchGame(selectedGame, "drawer");
    }
  });
  drawerTrailerPlay.addEventListener("click", playTrailer);

  refreshButton.addEventListener("click", () => {
    integrations.open("library");
  });
  addGameButton.addEventListener("click", openManualDialog);
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

  addLocalFolder.addEventListener("click", () => {
    void openFolderDialog("local");
  });
  addRomFolder.addEventListener("click", () => {
    void openFolderDialog("rom");
  });
  folderCancel.addEventListener("click", () => folderDialog.close());
  folderForm.addEventListener("submit", (event) => {
    event.preventDefault();
    void addFolder();
  });
  folderDialog.addEventListener("close", () => {
    folderPath.disabled = false;
    folderSubmit.disabled = false;
    folderBusy = false;
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
    void openFolderDialog("local");
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
  configureFolders: openFolderDialog,
  addGame: openManualDialog,
  reload: (discoverNew = false) => reloadAll(false, discoverNew),
  applyDownloads: applyDownloadedGames,
  error: messageOf,
  report: showToast,
  applyAppearance: (settings) => {
    uiSettings = applyUISettings(settings);
  },
  setViewMode: (mode) => setViewMode(mode, false),
});

element("open-integrations").addEventListener("click", () => integrations.open("plugins"));
element("open-metadata").addEventListener("click", () => integrations.openMetadata());

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
  toast: showToast,
  onGameUpdated: applyUpdatedGame,
});

element("details-launch-options").addEventListener("click", () => {
  if (detailsGame) properties.open(detailsGame, "general");
});
element("drawer-launch-options").addEventListener("click", () => {
  if (selectedGame) properties.open(selectedGame, "general");
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
