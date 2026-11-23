import { t } from "./i18n";
export function createBackgroundDownloads(errorMessage: (error: unknown) => string) {
  const panel = document.querySelector<HTMLElement>("#background-downloads")!;
  const bar = panel.querySelector<HTMLProgressElement>("progress")!;
  const status = panel.querySelector<HTMLElement>("[role=status]")!;
  const dismiss = panel.querySelector<HTMLButtonElement>("button")!;
  let queue = Promise.resolve();
  let batches = 0;
  let total = 0;
  let finished = 0;
  let failures = 0;
  let latestError = "";
  let active = "";
  let hideTimer = 0;

  function render(): void {
    const running = batches > 0;
    bar.max = Math.max(total, 1);
    bar.value = finished;
    bar.hidden = !running;
    dismiss.hidden = running;
    const text = running ? t("{0} · {1} of {2}", active || "Downloads", finished, total)
      : failures ? t("Downloads complete · {0} failed{1}", failures, failures === 1 ? "" : "s")
      : t("Downloads complete");
    status.textContent = text;
    panel.title = latestError ? `${text}. ${latestError}` : text;
    panel.classList.toggle("has-errors", failures > 0);
    panel.hidden = false;
    if (!running && !failures) {
      hideTimer = window.setTimeout(() => { panel.hidden = true; }, 4500);
    }
  }

  dismiss.addEventListener("click", () => { panel.hidden = true; });

  return {
    enqueue<T, Result>(
      label: string,
      items: T[],
      download: (item: T) => Promise<Result>,
      apply: (results: Result[]) => void | Promise<void>,
    ): void {
      if (!items.length) return;
      window.clearTimeout(hideTimer);
      if (batches === 0) {
        finished = total = failures = 0;
        latestError = "";
      }
      batches++;
      total += items.length;
      render();
      queue = queue.then(async () => {
        const results: Result[] = [];
        active = label;
        render();
        for (const item of items) {
          try {
            results.push(await download(item));
          } catch (error) {
            failures++;
            latestError = errorMessage(error);
          }
          finished++;
          if (finished < total) render();
        }
        try {
          if (results.length) await apply(results);
        } catch (error) {
          failures++;
          latestError = errorMessage(error);
        }
        batches--;
        active = "";
        render();
      });
    },
  };
}
