package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/playnux/core/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	key  []byte // chave AES-256 para as sessões das lojas
}

// Open inicializa a conexão SQLite com pragmas de alta velocidade e migração do schema.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir sqlite: %w", err)
	}

	if err := migrateDatabase(conn, path); err != nil {
		_ = conn.Close()
		return nil, err
	}

	key, err := sessionKey(path)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// O banco contém tokens de lojas: restringe o acesso ao usuário atual.
	if !isMemoryPath(path) {
		_ = os.Chmod(path, 0o600)
	}

	return &DB{conn: conn, key: key}, nil
}

// sessionKey usa uma chave persistida ao lado do banco. Em bancos em memória
// (usados em testes) gera uma chave efêmera.
func sessionKey(path string) ([]byte, error) {
	if isMemoryPath(path) {
		key := make([]byte, keySize)
		_, err := rand.Read(key)
		return key, err
	}
	return loadOrCreateKey(path + ".key")
}

func isMemoryPath(path string) bool {
	return path == ":memory:" || strings.Contains(path, "mode=memory")
}

// migrateGames adiciona colunas introduzidas depois da primeira versão do
// schema. Bancos novos já nascem completos; ALTER TABLE mantém os antigos.
func migrateGames(conn *sql.Tx) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"logo_image", "ALTER TABLE games ADD COLUMN logo_image TEXT"},
		{"trailer_url", "ALTER TABLE games ADD COLUMN trailer_url TEXT"},
		{"executable", "ALTER TABLE games ADD COLUMN executable TEXT"},
		{"arguments_json", "ALTER TABLE games ADD COLUMN arguments_json TEXT DEFAULT '[]'"},
	}
	for _, column := range columns {
		var count int
		err := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('games') WHERE name = ?`, column.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("verificar coluna %s: %w", column.name, err)
		}
		if count > 0 {
			continue
		}
		if _, err := conn.Exec(column.ddl); err != nil {
			return fmt.Errorf("migrar coluna %s: %w", column.name, err)
		}
	}
	return nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

// UpsertGames insere ou atualiza uma lista de jogos em uma única transação atômica rápida.
func (db *DB) UpsertGames(ctx context.Context, games []*models.Game) error {
	return db.upsertGames(ctx, games, false)
}

// UpsertStoreGames saves store metadata in the same transaction as the import.
func (db *DB) UpsertStoreGames(ctx context.Context, games []*models.Game) error {
	return db.upsertGames(ctx, games, true)
}

func (db *DB) upsertGames(ctx context.Context, games []*models.Game, recordStore bool) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT INTO games (
		id, game_id, plugin_id, source, name, sorting_name,
		icon, cover_image, background_image, logo_image,
		is_installed, install_directory, install_size,
		playtime, play_count, last_activity,
		description, release_date, release_year, version,
		user_score, critic_score, community_score,
		hidden, favorite, is_custom_game,
		genres_json, developers_json, publishers_json, tags_json,
		categories_json, features_json, platforms_json,
		trailer_url, executable, arguments_json,
		added, modified
	) VALUES (
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?,
		?, ?
	) ON CONFLICT(source, game_id) DO UPDATE SET
		name = excluded.name,
		sorting_name = excluded.sorting_name,
		icon = COALESCE(NULLIF(games.icon, ''), excluded.icon),
		cover_image = COALESCE(NULLIF(games.cover_image, ''), excluded.cover_image),
		background_image = COALESCE(NULLIF(games.background_image, ''), excluded.background_image),
		logo_image = COALESCE(NULLIF(games.logo_image, ''), excluded.logo_image),
		is_installed = excluded.is_installed,
		install_directory = excluded.install_directory,
		install_size = excluded.install_size,
		playtime = MAX(games.playtime, excluded.playtime),
		last_activity = COALESCE(excluded.last_activity, games.last_activity),
		trailer_url = COALESCE(NULLIF(games.trailer_url, ''), excluded.trailer_url),
		executable = COALESCE(NULLIF(excluded.executable, ''), games.executable),
		arguments_json = CASE WHEN excluded.arguments_json IN ('', '[]') THEN games.arguments_json ELSE excluded.arguments_json END,
		modified = CURRENT_TIMESTAMP;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("erro ao preparar upsert: %w", err)
	}
	defer stmt.Close()

	for _, g := range games {
		if g.ID == "" {
			g.ID = uuid.New().String()
		}
		if g.SortingName == "" {
			g.SortingName = g.Name
		}
		if g.Added.IsZero() {
			g.Added = time.Now()
		}
		g.Modified = time.Now()

		genresJSON, _ := json.Marshal(g.Genres)
		devsJSON, _ := json.Marshal(g.Developers)
		pubsJSON, _ := json.Marshal(g.Publishers)
		tagsJSON, _ := json.Marshal(g.Tags)
		catsJSON, _ := json.Marshal(g.Categories)
		featsJSON, _ := json.Marshal(g.Features)
		platsJSON, _ := json.Marshal(g.Platforms)
		argsJSON, _ := json.Marshal(g.Arguments)

		_, err = stmt.ExecContext(ctx,
			g.ID, g.GameID, g.PluginID, g.Source, g.Name, g.SortingName,
			g.Icon, g.CoverImage, g.BackgroundImage, g.LogoImage,
			g.IsInstalled, g.InstallDirectory, g.InstallSize,
			g.Playtime, g.PlayCount, g.LastActivity,
			g.Description, g.ReleaseDate, g.ReleaseYear, g.Version,
			g.UserScore, g.CriticScore, g.CommunityScore,
			g.Hidden, g.Favorite, g.IsCustomGame,
			string(genresJSON), string(devsJSON), string(pubsJSON), string(tagsJSON),
			string(catsJSON), string(featsJSON), string(platsJSON),
			g.TrailerURL, g.Executable, string(argsJSON),
			g.Added, g.Modified,
		)
		if err != nil {
			return fmt.Errorf("erro ao inserir jogo %s: %w", g.Name, err)
		}
		if recordStore {
			if err := saveStoreMetadata(ctx, tx, g); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// SearchGames realiza busca rápida usando a tabela virtual FTS5.
func (db *DB) SearchGames(ctx context.Context, term string, limit int) ([]*models.Game, error) {
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
	SELECT %s
	FROM games g
	JOIN games_fts fts ON g.id = fts.id
	WHERE games_fts MATCH ? AND g.hidden = 0
	ORDER BY g.sorting_name ASC
	LIMIT ?;
	`, gameColumnsFor("g"))

	rows, err := db.conn.QueryContext(ctx, query, term+"*", limit)
	if err != nil {
		return nil, fmt.Errorf("erro na busca FTS5: %w", err)
	}
	defer rows.Close()

	var result []*models.Game
	for rows.Next() {
		game, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, game)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
