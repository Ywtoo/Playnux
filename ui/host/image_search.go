package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/playnux/core/metadata"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func (h *Host) searchGoogleImages(ctx context.Context, query string, transparent bool) ([]metadata.WebImage, error) {
	searchURL, err := metadata.GoogleImageSearchURL(query, transparent)
	if err != nil {
		return nil, err
	}
	h.mu.RLock()
	app := h.app
	h.mu.RUnlock()
	if app == nil {
		return nil, fmt.Errorf("janela Wails ainda não inicializada")
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("iniciar seletor de imagens: %w", err)
	}
	defer listener.Close()

	results := make(chan []metadata.WebImage, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+token, func(w http.ResponseWriter, r *http.Request) {
		data := r.URL.Query().Get("results")
		if len(data) > 48<<10 {
			http.Error(w, "resultado excedeu o limite", http.StatusBadRequest)
			return
		}
		select {
		case results <- metadata.ParseGoogleImages(data):
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, MaxHeaderBytes: 256 << 10}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	reportURL := "http://" + listener.Addr().String() + "/" + token
	width, height, minWidth, minHeight := FitWindowToScreen(app, 1020, 740, 480, 520)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "playnux-image-search-" + token,
		Title:     "Playnux — Google Imagens",
		URL:       searchURL,
		JS:        googleResultsScript(reportURL),
		Width:     width,
		Height:    height,
		MinWidth:  minWidth,
		MinHeight: minHeight,
	})
	closed := make(chan struct{})
	var closeOnce sync.Once
	unregister := window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		closeOnce.Do(func() { close(closed) })
	})
	defer unregister()
	defer window.Close()
	window.Show()
	window.Center()
	window.Focus()

	timeout, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	select {
	case images := <-results:
		if len(images) > 0 {
			return images, nil
		}
	case <-closed:
		return nil, fmt.Errorf("busca fechada. Você também pode colar o endereço da imagem")
	case <-timeout.Done():
	}
	return nil, fmt.Errorf("o Google não retornou miniaturas. Abra a busca no navegador e cole o endereço da imagem")
}

func googleResultsScript(reportURL string) string {
	target, _ := json.Marshal(reportURL)
	pattern, _ := json.Marshal(metadata.GoogleImagePairPattern)
	return fmt.Sprintf(`(() => {
  if (location.hostname !== "www.google.com" || location.pathname !== "/search" || window.playnuxImageSearch) return;
  window.playnuxImageSearch = true;
  const timer = setInterval(() => {
    const rows = [];
    const pattern = new RegExp(%s, "g");
    for (const match of document.documentElement.outerHTML.matchAll(pattern)) {
      if (match[0].length <= 1800) rows.push(match[0]);
      if (rows.length >= 24) break;
    }
    for (const row of document.querySelectorAll(".rg_meta")) {
      if (rows.length >= 24) break;
      if (row.outerHTML.length <= 1800) rows.push(row.outerHTML);
    }
    if (!rows.length) {
      for (const image of document.images) {
        const width = image.naturalWidth || image.width;
        const height = image.naturalHeight || image.height;
        if (width < 64 || height < 48) continue;
        const candidates = [image.currentSrc, image.getAttribute("data-src"), image.src];
        const thumb = candidates.find(src => {
          try {
            const url = new URL(src);
            return url.protocol === "https:" && url.hostname.startsWith("encrypted-tbn") && url.hostname.endsWith(".gstatic.com");
          } catch { return false; }
        });
        if (!thumb) continue;
        const link = image.closest("a[href]");
        const original = link ? new URL(link.href, location.href).searchParams.get("imgurl") : null;
        const thumbnail = JSON.stringify([thumb, height, width]);
        const full = JSON.stringify([/^https?:\/\//i.test(original || "") ? original : thumb, height, width]);
        if (thumbnail.length + full.length <= 1800) rows.push(thumbnail + "," + full);
        if (rows.length >= 24) break;
      }
    }
    if (!rows.length) return;
    clearInterval(timer);
    location.replace(%s + "?results=" + encodeURIComponent(rows.join("\n")));
  }, 400);
})();`, pattern, target)
}
