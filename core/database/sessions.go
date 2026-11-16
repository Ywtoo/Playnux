package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/playnux/core/models"
)

// ErrSessionNotFound é retornado quando a loja ainda não possui sessão salva.
var ErrSessionNotFound = errors.New("sessão da loja não encontrada")

// SaveSession cifra e grava (ou atualiza) a sessão digital de uma loja.
func (db *DB) SaveSession(ctx context.Context, s *models.Session) error {
	if s == nil || s.PluginID == "" {
		return errors.New("sessão sem plugin_id")
	}
	if db.key == nil {
		return errors.New("armazenamento seguro indisponível")
	}

	s.UpdatedAt = time.Now()
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	payload, err := db.seal(raw)
	if err != nil {
		return fmt.Errorf("erro ao cifrar sessão: %w", err)
	}

	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO store_sessions (plugin_id, payload, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(plugin_id) DO UPDATE SET
			payload = excluded.payload,
			updated_at = CURRENT_TIMESTAMP;
	`, s.PluginID, payload)
	if err != nil {
		return fmt.Errorf("erro ao salvar sessão de %s: %w", s.PluginID, err)
	}
	return nil
}

// LoadSession lê e decifra a sessão digital salva de uma loja.
func (db *DB) LoadSession(ctx context.Context, pluginID string) (*models.Session, error) {
	var payload []byte
	err := db.conn.QueryRowContext(ctx,
		`SELECT payload FROM store_sessions WHERE plugin_id = ?`, pluginID,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}

	raw, err := db.openSealed(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao decifrar sessão de %s: %w", pluginID, err)
	}

	var s models.Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSession remove a sessão salva (logout).
func (db *DB) DeleteSession(ctx context.Context, pluginID string) error {
	_, err := db.conn.ExecContext(ctx,
		`DELETE FROM store_sessions WHERE plugin_id = ?`, pluginID)
	return err
}
