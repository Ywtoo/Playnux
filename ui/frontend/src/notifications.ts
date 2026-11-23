import { language, t } from "./i18n";
interface Notice {
  game: string;
  action: string;
  message: string;
  time: number;
}

const storageKey = "playnux:game-errors";
const limit = 50;

function restore(): Notice[] {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(storageKey) ?? "[]");
    if (!Array.isArray(raw)) return [];
    return raw.filter((item): item is Notice => item && typeof item.game === "string" &&
      typeof item.action === "string" && typeof item.message === "string" && Number.isFinite(item.time)).slice(0, limit);
  } catch {
    return [];
  }
}

export function setupNotifications(
  dialog: HTMLDialogElement,
  trigger: HTMLButtonElement,
  count: HTMLElement,
  list: HTMLElement,
  clear: HTMLButtonElement,
  close: HTMLButtonElement,
) {
  let notices = restore();
  const formatTime = new Intl.DateTimeFormat(language, { dateStyle: "short", timeStyle: "medium" });

  function render(): void {
    count.hidden = notices.length === 0;
    count.textContent = String(notices.length);
    trigger.setAttribute("aria-label", notices.length ? t("Game notifications ({0})", notices.length) : t("Game notifications"));
    clear.disabled = notices.length === 0;
    list.replaceChildren();
    if (!notices.length) {
      const empty = document.createElement("li");
      empty.textContent = t("No errors recorded.");
      list.append(empty);
      return;
    }
    for (const notice of notices) {
      const row = document.createElement("li");
      const heading = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = notice.game;
      const time = document.createElement("time");
      time.dateTime = new Date(notice.time).toISOString();
      time.textContent = formatTime.format(notice.time);
      const stage = document.createElement("span");
      stage.className = "notification-stage";
      stage.textContent = notice.action;
      const message = document.createElement("p");
      message.textContent = notice.message;
      heading.append(title, time);
      row.append(heading, stage, message);
      list.append(row);
    }
  }

  function save(): void {
    try {
      localStorage.setItem(storageKey, JSON.stringify(notices));
    } catch {
      // The current session can still display errors when storage is unavailable.
    }
    render();
  }

  trigger.addEventListener("click", () => dialog.showModal());
  close.addEventListener("click", () => dialog.close());
  clear.addEventListener("click", () => {
    notices = [];
    save();
  });
  render();

  return (game: string, action: string, message: string) => {
    notices.unshift({
      game: game.slice(0, 240),
      action,
      message: message.slice(0, 2500),
      time: Date.now(),
    });
    notices = notices.slice(0, limit);
    save();
  };
}
