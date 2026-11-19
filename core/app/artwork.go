package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/playnux/core/metadata"
)

// ServeArtwork expõe o cache local de imagens validado pelo Core.
func (c *App) ServeArtwork(next http.Handler) http.Handler {
	return c.artwork.ServeArtwork(next)
}

func (c *App) SearchProviderArtwork(ctx context.Context, query, field string) ([]metadata.WebImage, error) {
	return c.artwork.SearchProviderArtwork(ctx, query, field)
}

func (c *App) ApplySteamGridImage(ctx context.Context, id, field string, result metadata.WebImage) error {
	return c.artwork.ApplySteamGridImage(ctx, id, field, result)
}

func (c *App) ApplyProviderArtwork(ctx context.Context, id, field string) error {
	return c.artwork.ApplyProviderArtwork(ctx, id, field)
}

func (c *App) ApplyWebImage(ctx context.Context, id, field string, result metadata.WebImage) error {
	return c.artwork.ApplyWebImage(ctx, id, field, result)
}

// SaveGameArtwork grava uma imagem enviada pela UI no cache do Core.
func (c *App) SaveGameArtwork(ctx context.Context, id, field, encoded string) error {
	if len(encoded) > base64.StdEncoding.EncodedLen(3<<20) {
		return fmt.Errorf("a imagem deve ter até 3 MB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) > 3<<20 {
		return fmt.Errorf("imagem inválida ou maior que 3 MB")
	}
	path, err := c.artwork.StoreArtwork(data, "manual")
	if err != nil {
		return err
	}
	c.downloadMu.Lock()
	defer c.downloadMu.Unlock()
	return c.db.SetGameArtwork(ctx, id, field, path)
}
