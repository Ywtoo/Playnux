package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationBacksUpAnExistingLibraryAndKeepsItsGames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(SchemaSQL + `INSERT INTO games(id, game_id, plugin_id, source, name, sorting_name) VALUES ('fixture', '1', 'steam', 'Steam', 'Test Game', 'Test Game');`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	library, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	backupPath := path + ".schema-0.bak"
	backupInfo, err := os.Stat(backupPath)
	if err != nil || backupInfo.Mode().Perm() != 0o600 {
		t.Fatalf("backup missing or not private: %v", err)
	}
	backup, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	for version, database := range []*sql.DB{backup, library.conn} {
		var actualVersion int
		var title string
		if err := database.QueryRow(`PRAGMA user_version`).Scan(&actualVersion); err != nil || actualVersion != version {
			t.Fatalf("schema version %d: %d (%v)", version, actualVersion, err)
		}
		if err := database.QueryRow(`SELECT name FROM games WHERE id = 'fixture'`).Scan(&title); err != nil || title != "Test Game" {
			t.Fatalf("library title: %q (%v)", title, err)
		}
	}
}
