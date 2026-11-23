package host

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

var youtubeVideoID = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

var backgroundPlayer = template.Must(template.New("trailer").Parse(`<!doctype html>
<meta charset="utf-8">
<meta name="referrer" content="origin">
<style>
html,body{margin:0;width:100%;height:100%;overflow:hidden;background:transparent}
iframe{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);
  width:max(100vw,177.778vh);height:max(100vh,56.25vw);border:0;pointer-events:none}
</style>
<iframe id="player" title="Trailer" tabindex="-1" allow="autoplay; encrypted-media" src="{{.}}"></iframe>
<script>
function onYouTubeIframeAPIReady() {
  const initialSound = new URLSearchParams(location.search).get("sound") === "1";
  function reportSound(player) {
    parent.postMessage(player.isMuted() ? "playnux-trailer-sound-off" : "playnux-trailer-sound-on", "*");
  }
  new YT.Player("player", {events: {
    onReady: function(event) {
      const player = event.target;
      if (initialSound) player.unMute();
      else player.mute();
      window.addEventListener("message", function(message) {
        if (message.source !== parent || typeof message.data?.trailerSound !== "boolean") return;
        if (message.data.trailerSound) player.unMute();
        else player.mute();
        reportSound(player);
        if (player.getPlayerState() !== YT.PlayerState.PLAYING) player.playVideo();
      });
      player.playVideo();
    },
    onStateChange: function(event) {
      const playing = event.data === YT.PlayerState.PLAYING;
      parent.postMessage(playing ? "playnux-trailer-playing" : "playnux-trailer-idle", "*");
      if (playing) reportSound(event.target);
    },
    onError: function() { parent.postMessage("playnux-trailer-error", "*"); },
    onAutoplayBlocked: function(event) {
      if (event.target.isMuted()) parent.postMessage("playnux-trailer-error", "*");
      else {
        event.target.mute();
        event.target.playVideo();
      }
    }
  }});
}
</script>
<script src="https://www.youtube.com/iframe_api" async></script>`))

// GetYouTubeBackgroundURL serves the iframe player from an HTTP origin so
// YouTube receives a Referer even when the desktop UI uses the wails scheme.
func (h *Host) GetYouTubeBackgroundURL(id string) (string, error) {
	if !youtubeVideoID.MatchString(id) {
		return "", fmt.Errorf("vídeo do YouTube inválido")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.trailerURL == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		address := listener.Addr().String()
		origin := "http://" + address
		mux := http.NewServeMux()
		mux.HandleFunc("GET /trailer", func(w http.ResponseWriter, r *http.Request) {
			video := r.URL.Query().Get("v")
			if r.Host != address || !youtubeVideoID.MatchString(video) {
				http.NotFound(w, r)
				return
			}
			query := url.Values{
				"origin": {origin}, "enablejsapi": {"1"},
				"controls": {"0"}, "disablekb": {"1"}, "fs": {"0"}, "playsinline": {"1"},
				"loop": {"1"}, "playlist": {video},
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "origin")
			_ = backgroundPlayer.Execute(w, "https://www.youtube-nocookie.com/embed/"+video+"?"+query.Encode())
		})
		h.trailerServer = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		h.trailerURL = origin
		server := h.trailerServer
		go func() { _ = server.Serve(listener) }()
	}
	return h.trailerURL + "/trailer?v=" + id, nil
}
