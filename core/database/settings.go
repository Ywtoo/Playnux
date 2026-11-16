package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrSettingNotFound indica que a preferência ainda não foi definida.
var ErrSettingNotFound = errors.New("preferência não encontrada")

// GetSetting lê uma preferência persistida. Valores ausentes devolvem
// ErrSettingNotFound para o chamador decidir o padrão.
func (db *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := db.conn.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?;`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSettingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("ler preferência %s: %w", key, err)
	}
	return value, nil
}

// SetSetting grava ou substitui uma preferência.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.conn.ExecContext(ctx, `
INSERT INTO settings (key, value, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
	value = excluded.value,
	updated_at = CURRENT_TIMESTAMP;`, key, value)
	if err != nil {
		return fmt.Errorf("salvar preferência %s: %w", key, err)
	}
	return nil
}
