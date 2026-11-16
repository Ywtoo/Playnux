package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/playnux/core/models"
)

func TestSessionRoundTripIsEncrypted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playnux.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	session := &models.Session{
		PluginID:     "epic",
		AccountID:    "account-1",
		AccessToken:  "super-secret-access-token",
		RefreshToken: "super-secret-refresh-token",
	}

	if err := db.SaveSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.LoadSession(ctx, "epic")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != session.AccessToken || loaded.RefreshToken != session.RefreshToken {
		t.Fatalf("sessão carregada incorreta: %+v", loaded)
	}

	var payload []byte
	if err := db.conn.QueryRow(`SELECT payload FROM store_sessions WHERE plugin_id = ?`, "epic").
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "super-secret") {
		t.Fatal("o payload salvo contém tokens em texto puro")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissões do banco = %o, esperava 600", info.Mode().Perm())
	}

	if err := db.DeleteSession(ctx, "epic"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LoadSession(ctx, "epic"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("esperava ErrSessionNotFound, obtive %v", err)
	}
}
