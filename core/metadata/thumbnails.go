package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const thumbPrefix = "/artwork/thumb/"

var thumbnailSizes = map[string]image.Point{
	"icon":        {X: 48, Y: 48},
	"cover":       {X: 250, Y: 375},
	"cover-hd":    {X: 500, Y: 750},
	"banner":      {X: 1440, Y: 810},
	"banner-hd":   {X: 1920, Y: 1080},
	"big-banner":  {X: 3840, Y: 2160},
	"logo":        {X: 860, Y: 300},
	"logo-hd":     {X: 1440, Y: 480},
	"carousel":    {X: 240, Y: 360},
	"carousel-hd": {X: 480, Y: 720},
}

var thumbnailSlots = make(chan struct{}, 3)
var thumbnailLocks [32]sync.Mutex

func artworkCDN(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "steamcdn-a.akamaihd.net", "cdn.cloudflare.steamstatic.com", "images.igdb.com", "images.gog.com":
		return true
	}
	for _, domain := range []string{"steamstatic.com", "steamgriddb.com", "gog-statics.com", "epicgames.com", "unrealengine.com"} {
		if strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

var thumbnailClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !artworkCDN(req.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func (m *ArtworkManager) serveThumbnail(w http.ResponseWriter, r *http.Request, role string) {
	bounds, ok := thumbnailSizes[role]
	source := r.URL.Query().Get("src")
	if !ok || m.Dir == "" || len(source) == 0 || len(source) > 4096 || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	local, isLocal := strings.CutPrefix(source, artworkPrefix)
	if isLocal {
		if !artworkFilename.MatchString(local) {
			http.NotFound(w, r)
			return
		}
	} else if u, err := url.Parse(source); err != nil || !artworkCDN(u) {
		http.NotFound(w, r)
		return
	}

	version := "v1"
	trimTransparent := role == "logo" || role == "logo-hd"
	if trimTransparent {
		version = "v2"
	}
	hash := sha256.Sum256([]byte(version + "\x00" + role + "\x00" + source))
	target := filepath.Join(m.Dir, "thumbnails", fmt.Sprintf("%x", hash))
	ready := func() bool {
		info, err := os.Stat(target)
		return err == nil && (isLocal || time.Since(info.ModTime()) < 7*24*time.Hour)
	}
	serve := func() {
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, target)
	}
	if ready() {
		serve()
		return
	}

	lock := &thumbnailLocks[int(hash[0])%len(thumbnailLocks)]
	lock.Lock()
	defer lock.Unlock()
	if ready() {
		serve()
		return
	}
	select {
	case thumbnailSlots <- struct{}{}:
		defer func() { <-thumbnailSlots }()
	case <-r.Context().Done():
		return
	}

	data, err := m.readThumbnailSource(r.Context(), source, local, isLocal)
	if err == nil {
		err = writeThumbnail(target, data, bounds, trimTransparent, role == "big-banner")
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serve()
}

func (m *ArtworkManager) readThumbnailSource(ctx context.Context, source, local string, isLocal bool) ([]byte, error) {
	var reader io.ReadCloser
	if isLocal {
		file, err := os.Open(filepath.Join(m.Dir, local))
		if err != nil {
			return nil, err
		}
		reader = file
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "image/*")
		response, err := thumbnailClient.Do(req)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("imagem indisponível")
		}
		reader = response.Body
	}
	defer reader.Close()
	const maxBytes = 12 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if len(data) > maxBytes {
		return nil, fmt.Errorf("imagem excede o limite")
	}
	return data, err
}

func writeThumbnail(target string, data []byte, limits image.Point, trimTransparent, highQuality bool) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 ||
		config.Height > 8192 || int64(config.Width)*int64(config.Height) > 24_000_000 {
		return fmt.Errorf("dimensões da imagem inválidas")
	}
	original, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	source := original.Bounds()
	if trimTransparent {
		source = visibleLogoBounds(original)
	}
	width, height := float64(source.Dx()), float64(source.Dy())
	scale := min(1.0, float64(limits.X)/width, float64(limits.Y)/height)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	output, err := os.CreateTemp(dir, ".thumb-")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	if highQuality && scale == 1 && format != "gif" {
		_, err = output.Write(data)
	} else {
		resized := image.NewNRGBA(image.Rect(0, 0, max(1, int(width*scale)), max(1, int(height*scale))))
		draw.CatmullRom.Scale(resized, resized.Bounds(), original, source, draw.Src, nil)
		if resized.Opaque() {
			quality := 85
			if highQuality {
				quality = 94
			}
			err = jpeg.Encode(output, resized, &jpeg.Options{Quality: quality})
		} else {
			err = png.Encode(output, resized)
		}
	}
	closeErr := output.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(output.Name(), target)
}

func visibleLogoBounds(logo image.Image) image.Rectangle {
	bounds := logo.Bounds()
	if opaque, ok := logo.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		return bounds
	}
	visible := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := logo.At(x, y).RGBA()
			if alpha != 0 {
				visible.Min.X = min(visible.Min.X, x)
				visible.Min.Y = min(visible.Min.Y, y)
				visible.Max.X = max(visible.Max.X, x+1)
				visible.Max.Y = max(visible.Max.Y, y+1)
			}
		}
	}
	if visible.Empty() {
		return bounds
	}
	return visible
}
