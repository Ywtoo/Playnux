package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/playnux/core/models"
)

func storeMetadataFields(game *models.Game) map[string]any {
	fields := make(map[string]any)
	for key, value := range map[string]string{
		"description": game.Description, "icon": game.Icon, "cover_image": game.CoverImage,
		"background_image": game.BackgroundImage, "logo_image": game.LogoImage, "trailer_url": game.TrailerURL,
	} {
		if value != "" {
			fields[key] = value
		}
	}
	for key, value := range map[string][]string{
		"genres": game.Genres, "developers": game.Developers, "publishers": game.Publishers,
		"platforms": game.Platforms, "tags": game.Tags, "features": game.Features,
	} {
		if len(value) > 0 {
			fields[key] = value
		}
	}
	if game.ReleaseDate != nil {
		fields["release_date"] = game.ReleaseDate
	}
	if game.ReleaseYear > 0 {
		fields["release_year"] = game.ReleaseYear
	}
	if game.CriticScore != nil {
		fields["critic_score"] = game.CriticScore
	}
	if game.CommunityScore != nil {
		fields["community_score"] = game.CommunityScore
	}
	return fields
}

func saveStoreMetadata(ctx context.Context, tx *sql.Tx, game *models.Game) error {
	switch game.PluginID {
	case "steam", "gog", "epic", "xbox":
	default:
		return nil
	}
	fields := storeMetadataFields(game)
	if len(fields) == 0 {
		return nil
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO store_game_metadata (plugin_id, game_id, payload) VALUES (?, ?, ?)
		ON CONFLICT(plugin_id, game_id) DO UPDATE SET payload = json_patch(store_game_metadata.payload, excluded.payload)
	`, game.PluginID, game.GameID, string(payload))
	return err
}

// StoreMetadata returns the latest available original values reported by the store.
func (db *DB) StoreMetadata(ctx context.Context, pluginID, gameID string) (*models.Game, error) {
	var payload string
	err := db.conn.QueryRowContext(ctx, "SELECT payload FROM store_game_metadata WHERE plugin_id = ? AND game_id = ?", pluginID, gameID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var game models.Game
	if err := json.Unmarshal([]byte(payload), &game); err != nil {
		return nil, err
	}
	return &game, nil
}
