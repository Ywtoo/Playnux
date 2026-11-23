package steam

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type storeMovie struct {
	Highlight bool              `json:"highlight"`
	HLS       string            `json:"hls_h264"`
	MP4       map[string]string `json:"mp4"`
	WebM      map[string]string `json:"webm"`
}

// TrailerSources returns direct storefront video URLs in playback order.
func (s *SteamProvider) TrailerSources(ctx context.Context, appID string, highQuality bool) []string {
	if id, err := strconv.ParseUint(appID, 10, 32); err != nil || id == 0 {
		return nil
	}
	query := url.Values{"appids": {appID}, "filters": {"movies"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://store.steampowered.com/api/appdetails?"+query.Encode(), nil)
	if err != nil {
		return nil
	}
	response, err := (&http.Client{Timeout: 6 * time.Second}).Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	return parseTrailerSources(io.LimitReader(response.Body, 2<<20), appID, highQuality)
}

func parseTrailerSources(body io.Reader, appID string, highQuality bool) []string {
	var payload map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Movies []storeMovie `json:"movies"`
		} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil
	}
	app := payload[appID]
	if !app.Success {
		return nil
	}
	for _, highlighted := range []bool{true, false} {
		for _, movie := range app.Data.Movies {
			if movie.Highlight != highlighted {
				continue
			}
			var sources []string
			order := []string{movie.MP4["480"], movie.MP4["max"], movie.WebM["480"], movie.WebM["max"], movie.HLS}
			if highQuality {
				order = []string{movie.MP4["max"], movie.WebM["max"], movie.HLS, movie.MP4["480"], movie.WebM["480"]}
			}
			for _, raw := range order {
				if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") {
					parsed.Scheme = "https"
					sources = append(sources, parsed.String())
				}
			}
			if len(sources) > 0 {
				return sources
			}
		}
	}
	return nil
}
