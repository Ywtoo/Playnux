package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

var migrations = []func(*sql.Tx) error{initializeLibrary}

func migrateDatabase(db *sql.DB, path string) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL; PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("configure library database: %w", err)
	}
	var version, tables int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("this library requires a newer version of Playnux")
	}
	if version == len(migrations) {
		return nil
	}
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name = 'games' AND type = 'table'`).Scan(&tables); err != nil {
		return err
	}
	if tables != 0 && !isMemoryPath(path) {
		if err := backupBeforeMigration(ctx, conn, path, version); err != nil {
			return fmt.Errorf("back up library before migration: %w", err)
		}
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("this library requires a newer version of Playnux")
	}
	for index := version; index < len(migrations); index++ {
		if err := migrations[index](tx); err != nil {
			return fmt.Errorf("migrate library schema to version %d: %w", index+1, err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
		return err
	}
	return tx.Commit()
}

func initializeLibrary(tx *sql.Tx) error {
	if _, err := tx.Exec(SchemaSQL); err != nil {
		return err
	}
	if err := migrateGames(tx); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO games_fts(games_fts) VALUES('rebuild')`)
	return err
}

func backupBeforeMigration(ctx context.Context, conn *sql.Conn, path string, version int) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".playnux-backup-*.db")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, file.Name()); err != nil {
		return err
	}
	return os.Rename(file.Name(), fmt.Sprintf("%s.schema-%d.bak", path, version))
}
