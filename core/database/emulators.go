package database

import (
	"context"
	"fmt"
)

// EmulatorRecord is a custom emulator persisted in SQLite. Profiles are stored
// as JSON so the Core owns their schema.
type EmulatorRecord struct {
	ID           string
	Name         string
	Executable   string
	FlatpakID    string
	ProfilesJSON string
}

// ListCustomEmulators returns user-defined emulators ordered by name.
func (db *DB) ListCustomEmulators(ctx context.Context) ([]EmulatorRecord, error) {
	rows, err := db.conn.QueryContext(ctx, `
SELECT id, name, executable, flatpak_id, profiles_json
FROM emulators ORDER BY name COLLATE NOCASE ASC;`)
	if err != nil {
		return nil, fmt.Errorf("listar emuladores: %w", err)
	}
	defer rows.Close()

	var result []EmulatorRecord
	for rows.Next() {
		var record EmulatorRecord
		if err := rows.Scan(&record.ID, &record.Name, &record.Executable, &record.FlatpakID, &record.ProfilesJSON); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// SaveCustomEmulator inserts or updates a custom emulator definition.
func (db *DB) SaveCustomEmulator(ctx context.Context, record EmulatorRecord) error {
	_, err := db.conn.ExecContext(ctx, `
INSERT INTO emulators (id, name, executable, flatpak_id, profiles_json, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    executable = excluded.executable,
    flatpak_id = excluded.flatpak_id,
    profiles_json = excluded.profiles_json,
    updated_at = CURRENT_TIMESTAMP;`,
		record.ID, record.Name, record.Executable, record.FlatpakID, record.ProfilesJSON)
	if err != nil {
		return fmt.Errorf("salvar emulador %s: %w", record.ID, err)
	}
	return nil
}

// DeleteCustomEmulator removes a user-defined emulator.
func (db *DB) DeleteCustomEmulator(ctx context.Context, id string) error {
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM emulators WHERE id = ?;`, id); err != nil {
		return fmt.Errorf("remover emulador %s: %w", id, err)
	}
	return nil
}
