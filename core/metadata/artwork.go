package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/playnux/core/models"
	_ "golang.org/x/image/webp"
)

const maxArtworkBytes = 3 << 20
const artworkPrefix = "/artwork/"

var artworkFilename = regexp.MustCompile(`^(manual|provider)-[a-f0-9]{64}\.(png|jpeg|gif|webp)$`)

// ArtworkStore é a parte do banco usada pelo gerenciador de imagens.
type ArtworkStore interface {
	SetGameArtwork(ctx context.Context, id, field, image string) error
}

// ArtworkManager valida, baixa e grava as capas do Playnux no disco local.
// A busca em provedores é injetada pelo Core para que este pacote não dependa
// dos clientes concretos (e não crie ciclo de importação).
type ArtworkManager struct {
	Dir        string
	DB         ArtworkStore
	DownloadMu *sync.Mutex
	Search     func(ctx context.Context, query, field string) ([]WebImage, error)
	Lookup     func(ctx context.Context, name, field string) (string, error)
}

func (m *ArtworkManager) StoreArtwork(data []byte, origin string) (string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width > 8192 || config.Height > 8192 || config.Width <= 0 || config.Height <= 0 {
		return "", fmt.Errorf("use uma imagem PNG, JPG, GIF ou WebP de até 8192 pixels por lado")
	}
	if m.Dir == "" {
		return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%x.%s", origin, sha256.Sum256(data), format)
	target := filepath.Join(m.Dir, name)
	if _, err := os.Stat(target); err == nil {
		return artworkPrefix + name, nil
	}
	tmp, err := os.CreateTemp(m.Dir, ".image-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	return artworkPrefix + name, nil
}

// ServeArtwork exposes validated image filenames from Playnux's private cache.
func (m *ArtworkManager) ServeArtwork(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role, ok := strings.CutPrefix(r.URL.Path, thumbPrefix); ok {
			m.serveThumbnail(w, r, role)
			return
		}
		name, ok := strings.CutPrefix(r.URL.Path, artworkPrefix)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if m.Dir == "" || !artworkFilename.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, filepath.Join(m.Dir, name))
	})
}

func providerArtworkURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Host == "cdn.steamgriddb.com" || u.Host == "cdn2.steamgriddb.com")
}

var artworkClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !providerArtworkURL(req.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func (m *ArtworkManager) CacheProviderArtwork(ctx context.Context, before models.Game, game *models.Game) {
	if m.Dir == "" {
		return
	}
	for _, field := range []struct {
		previous string
		current  *string
	}{
		{before.CoverImage, &game.CoverImage},
		{before.BackgroundImage, &game.BackgroundImage},
		{before.LogoImage, &game.LogoImage},
		{before.Icon, &game.Icon},
	} {
		if field.previous != *field.current {
			*field.current = m.CacheArtworkURL(ctx, *field.current)
		}
	}
}

func (m *ArtworkManager) CacheArtworkURL(ctx context.Context, source string) string {
	path, err := m.DownloadProviderArtwork(ctx, source)
	if err != nil {
		return source
	}
	return path
}

func (m *ArtworkManager) DownloadProviderArtwork(ctx context.Context, source string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil || !providerArtworkURL(req.URL) {
		return "", fmt.Errorf("endereço de arte do SteamGridDB inválido")
	}
	req.Header.Set("Accept", "image/*")
	resp, err := artworkClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("arte indisponível no SteamGridDB")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtworkBytes+1))
	if err != nil || len(data) > maxArtworkBytes {
		return "", fmt.Errorf("imagem ilegível ou maior que 3 MB")
	}
	return m.StoreArtwork(data, "provider")
}

func (m *ArtworkManager) SearchProviderArtwork(ctx context.Context, query, field string) ([]WebImage, error) {
	if m.Search == nil {
		return nil, fmt.Errorf("cadastre a chave em Configurações → SteamGridDB")
	}
	if query = strings.TrimSpace(query); query == "" || len(query) > 400 {
		return nil, fmt.Errorf("informe uma busca de até 400 bytes")
	}
	return m.Search(ctx, query, field)
}

func (m *ArtworkManager) ApplySteamGridImage(ctx context.Context, id, field string, result WebImage) error {
	if m.DB == nil {
		return fmt.Errorf("banco de imagens indisponível")
	}
	for _, source := range []string{result.URL, result.Thumbnail} {
		path, err := m.DownloadProviderArtwork(ctx, source)
		if err == nil {
			return m.setArtwork(ctx, id, field, path)
		}
		if result.URL == result.Thumbnail {
			break
		}
	}
	return fmt.Errorf("não foi possível salvar a imagem do SteamGridDB")
}

// ApplyProviderArtwork uses the existing SteamGridDB credential for one explicit edit.
func (m *ArtworkManager) ApplyProviderArtwork(ctx context.Context, id, field string) error {
	if m.Lookup == nil || m.DB == nil {
		return fmt.Errorf("cadastre a chave em Configurações → SteamGridDB")
	}
	source, err := m.Lookup(ctx, id, field)
	if err != nil {
		return err
	}
	path := m.CacheArtworkURL(ctx, source)
	return m.setArtwork(ctx, id, field, path)
}

func (m *ArtworkManager) setArtwork(ctx context.Context, id, field, path string) error {
	if m.DownloadMu != nil {
		m.DownloadMu.Lock()
		defer m.DownloadMu.Unlock()
	}
	return m.DB.SetGameArtwork(ctx, id, field, path)
}
