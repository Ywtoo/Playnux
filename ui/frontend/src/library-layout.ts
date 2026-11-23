import { t } from "./i18n";
const sidebarMin = 185;
const sidebarMax = 440;

export function configureDetailsSidebar(view: HTMLElement, button: HTMLButtonElement): void {
  const sidebar = view.querySelector<HTMLElement>("#details-sidebar")!;
  const handle = view.querySelector<HTMLElement>("#details-sidebar-resize")!;
  const widthKey = "playnux:details-sidebar-width";
  const visibilityKey = "playnux:details-sidebar-hidden";

  function resize(width: number): void {
    const size = Math.round(Math.min(sidebarMax, Math.max(sidebarMin, width)));
    view.style.setProperty("--details-sidebar-width", `${size}px`);
    handle.setAttribute("aria-valuenow", String(size));
  }

  function setHidden(hidden: boolean): void {
    sidebar.hidden = handle.hidden = hidden;
    view.classList.toggle("is-sidebar-hidden", hidden);
    button.setAttribute("aria-expanded", String(!hidden));
    button.title = hidden ? t("Show game sidebar") : t("Hide game sidebar");
  }

  const stored = Number(localStorage.getItem(widthKey));
  if (stored > 0) resize(stored);
  else handle.setAttribute("aria-valuenow", "302");
  setHidden(localStorage.getItem(visibilityKey) === "true");
  button.addEventListener("click", () => {
    setHidden(!sidebar.hidden);
    localStorage.setItem(visibilityKey, String(sidebar.hidden));
  });

  handle.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    handle.setPointerCapture(event.pointerId);
  });
  handle.addEventListener("pointermove", (event) => {
    if (handle.hasPointerCapture(event.pointerId)) resize(event.clientX - view.getBoundingClientRect().left);
  });
  function saveWidth(): void {
    localStorage.setItem(widthKey, handle.getAttribute("aria-valuenow")!);
  }
  handle.addEventListener("lostpointercapture", saveWidth);
  handle.addEventListener("keydown", (event) => {
    const width = sidebar.getBoundingClientRect().width;
    const next = { ArrowLeft: width - 16, ArrowRight: width + 16, Home: sidebarMin, End: sidebarMax }[event.key];
    if (next === undefined) return;
    event.preventDefault();
    resize(next);
    saveWidth();
  });
}

export function configureGridCards(grid: HTMLElement): void {
  const options = document.querySelector<HTMLDetailsElement>("#grid-card-options")!;
  const ratio = options.querySelector<HTMLSelectElement>("#grid-cover-ratio")!;
  const fields = options.querySelectorAll<HTMLInputElement>("[data-grid-field]");
  const ratioKey = "playnux:grid-cover-ratio";
  const stored = localStorage.getItem(ratioKey);
  if (stored && [...ratio.options].some((option) => option.value === stored)) ratio.value = stored;
  function apply(): void {
    grid.dataset.ratio = ratio.value;
    for (const field of fields) grid.classList.toggle(`hide-${field.dataset.gridField}`, !field.checked);
  }
  for (const field of fields) field.checked = localStorage.getItem(`playnux:grid-field:${field.dataset.gridField}`) !== "off";
  apply();
  options.addEventListener("change", () => {
    apply();
    localStorage.setItem(ratioKey, ratio.value);
    for (const field of fields) localStorage.setItem(`playnux:grid-field:${field.dataset.gridField}`, field.checked ? "on" : "off");
  });
  options.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && options.open) {
      options.open = false;
      options.querySelector("summary")?.focus();
    }
  });
  options.addEventListener("focusout", (event) => {
    if (event.relatedTarget instanceof Node && !options.contains(event.relatedTarget)) options.open = false;
  });
  grid.addEventListener("keydown", (event) => {
    if (!(event.target instanceof HTMLElement) || !event.target.classList.contains("game-card")) return;
    if (!event.key.startsWith("Arrow")) return;
    const columns = getComputedStyle(grid).gridTemplateColumns.split(" ").length;
    const step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -columns, ArrowDown: columns }[event.key];
    if (step === undefined) return;
    const cards = [...grid.children];
    const index = cards.indexOf(event.target.parentElement!);
    const next = cards[index + step]?.querySelector<HTMLElement>(".game-card");
    event.preventDefault();
    next?.focus();
  });
}
