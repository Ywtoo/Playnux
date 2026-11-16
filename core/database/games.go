package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/playnux/core/models"
)

// ErrGameNotFound é retornado quando nenhum registro corresponde ao ID.
var ErrGameNotFound = errors.New("jogo não encontrado")

const gameColumns = `
	id, game_id, plugin_id, source, name, sorting_name,
	icon, cover_image, background_image, logo_image, trailer_url,
	is_installed, install_directory, install_size, executable, arguments_json,
	playtime, play_count, last_activity,
	description, release_date, release_year, version,
	user_score, critic_score, community_score,
	hidden, favorite, is_custom_game,
	genres_json, developers_json, publishers_json, tags_json,
	categories_json, features_json, platforms_json,
	added, modified
`

type rowScanner interface {
	Scan(dest ...any) error
}

// gameColumnsFor prefixa cada coluna com o alias da tabela. JOINs (busca FTS)
// precisam disso para evitar ambiguidade entre games e games_fts.
func gameColumnsFor(alias string) string {
	parts := strings.Split(gameColumns, ",")
	for index, part := range parts {
		parts[index] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func scanGame(row rowScanner) (*models.Game, error) {
	var game models.Game
	// Colunas de texto são anuláveis no SQLite. Bancos migrados de versões
	// anteriores recebem NULL nas colunas novas, então usamos NullString em vez
	// de deixar o scan falhar.
	var icon, cover, background, logo, trailer sql.NullString
	var installDir, executable, arguments sql.NullString
	var description, version sql.NullString
	var installSize, playtime, playCount, releaseYear sql.NullInt64
	var genres, developers, publishers, tags, categories, features, platforms sql.NullString

	err := row.Scan(
		&game.ID, &game.GameID, &game.PluginID, &game.Source, &game.Name, &game.SortingName,
		&icon, &cover, &background, &logo, &trailer,
		&game.IsInstalled, &installDir, &installSize, &executable, &arguments,
		&playtime, &playCount, &game.LastActivity,
		&description, &game.ReleaseDate, &releaseYear, &version,
		&game.UserScore, &game.CriticScore, &game.CommunityScore,
		&game.Hidden, &game.Favorite, &game.IsCustomGame,
		&genres, &developers, &publishers, &tags,
		&categories, &features, &platforms,
		&game.Added, &game.Modified,
	)
	if err != nil {
		return nil, err
	}

	game.Icon = icon.String
	game.CoverImage = cover.String
	game.BackgroundImage = background.String
	game.LogoImage = logo.String
	game.TrailerURL = trailer.String
	game.InstallDirectory = installDir.String
	game.InstallSize = uint64(installSize.Int64)
	game.Executable = executable.String
	game.Playtime = uint64(playtime.Int64)
	game.PlayCount = int(playCount.Int64)
	game.Description = description.String
	game.ReleaseYear = int(releaseYear.Int64)
	game.Version = version.String
	game.Arguments = decodeList(arguments.String)
	game.Genres = decodeList(genres.String)
	game.Developers = decodeList(developers.String)
	game.Publishers = decodeList(publishers.String)
	game.Tags = decodeList(tags.String)
	game.Categories = decodeList(categories.String)
	game.Features = decodeList(features.String)
	game.Platforms = decodeList(platforms.String)
	return &game, nil
}

func decodeList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

// ListGames devolve a biblioteca visível para a UI. O filtro aceita "all",
// "installed", "favorites" e "recent"; valores desconhecidos caem em "all".
func (db *DB) ListGames(ctx context.Context, filter string, limit int) ([]*models.Game, error) {
	if limit <= 0 {
		limit = 1000
	}

	where := "hidden = 0"
	order := "sorting_name ASC"
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "installed":
		where += " AND is_installed = 1"
	case "favorites":
		where += " AND favorite = 1"
	case "recent":
		where += " AND last_activity IS NOT NULL"
		order = "last_activity DESC"
	}

	query := fmt.Sprintf(`SELECT %s FROM games WHERE %s ORDER BY %s LIMIT ?;`,
		gameColumns, where, order)
	rows, err := db.conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar jogos: %w", err)
	}
	defer rows.Close()

	var games []*models.Game
	for rows.Next() {
		game, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	return games, rows.Err()
}

// GetGame carrega todos os metadados de um jogo pelo ID interno.
func (db *DB) GetGame(ctx context.Context, id string) (*models.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM games WHERE id = ?;`, gameColumns)
	game, err := scanGame(db.conn.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGameNotFound
	}
	if err != nil {
		return nil, err
	}
	return game, nil
}

// DeleteGamesByGameIDPrefix remove jogos de uma fonte cujo GameID comece pelo
// prefixo informado. É usado ao remover uma pasta local/ROM para não deixar
// cartões órfãos na biblioteca.
func (db *DB) DeleteGamesByGameIDPrefix(ctx context.Context, source, prefix string) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
DELETE FROM games
WHERE source = ? AND substr(game_id, 1, length(?)) = ?;`, source, prefix, prefix)
	if err != nil {
		return 0, fmt.Errorf("remover jogos de %s: %w", source, err)
	}
	return result.RowsAffected()
}

// UpdateGameMetadata grava apenas os campos enriquecidos por provedores de
// metadados, preservando estado de instalação, progresso e flags do usuário.
func (db *DB) UpdateGameMetadata(ctx context.Context, game *models.Game) error {
	if game == nil || game.ID == "" {
		return fmt.Errorf("jogo inválido para atualizar metadados")
	}

	genresJSON, _ := json.Marshal(game.Genres)
	devsJSON, _ := json.Marshal(game.Developers)
	pubsJSON, _ := json.Marshal(game.Publishers)
	featuresJSON, _ := json.Marshal(game.Features)
	tagsJSON, _ := json.Marshal(game.Tags)
	platformsJSON, _ := json.Marshal(game.Platforms)

	result, err := db.conn.ExecContext(ctx, `
UPDATE games SET
	description = COALESCE(NULLIF(?, ''), description),
	release_date = COALESCE(?, release_date),
	release_year = CASE WHEN ? > 0 THEN ? ELSE release_year END,
	critic_score = COALESCE(?, critic_score),
	community_score = COALESCE(?, community_score),
	genres_json = CASE WHEN ? IN ('', '[]', 'null') THEN genres_json ELSE ? END,
	developers_json = CASE WHEN ? IN ('', '[]', 'null') THEN developers_json ELSE ? END,
	publishers_json = CASE WHEN ? IN ('', '[]', 'null') THEN publishers_json ELSE ? END,
	tags_json = CASE WHEN ? IN ('', '[]', 'null') THEN tags_json ELSE ? END,
	features_json = CASE WHEN ? IN ('', '[]', 'null') THEN features_json ELSE ? END,
	platforms_json = CASE WHEN ? IN ('', '[]', 'null') THEN platforms_json ELSE ? END,
	trailer_url = COALESCE(NULLIF(?, ''), trailer_url),
	icon = COALESCE(NULLIF(?, ''), icon),
	cover_image = COALESCE(NULLIF(?, ''), cover_image),
	background_image = COALESCE(NULLIF(?, ''), background_image),
	logo_image = COALESCE(NULLIF(?, ''), logo_image),
	modified = CURRENT_TIMESTAMP
WHERE id = ?;`,
		game.Description,
		game.ReleaseDate,
		game.ReleaseYear, game.ReleaseYear,
		game.CriticScore, game.CommunityScore,
		string(genresJSON), string(genresJSON),
		string(devsJSON), string(devsJSON),
		string(pubsJSON), string(pubsJSON),
		string(tagsJSON), string(tagsJSON),
		string(featuresJSON), string(featuresJSON),
		string(platformsJSON), string(platformsJSON),
		game.TrailerURL,
		game.Icon,
		game.CoverImage,
		game.BackgroundImage,
		game.LogoImage,
		game.ID,
	)
	if err != nil {
		return fmt.Errorf("atualizar metadados de %s: %w", game.Name, err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrGameNotFound
	}
	return nil
}

// UpdateGameDetails grava os campos editáveis na janela de propriedades.
// Diferente de UpdateGameMetadata, campos vazios são limpos de propósito: o
// usuário está confirmando o estado final do formulário.
func (db *DB) UpdateGameDetails(ctx context.Context, game *models.Game) error {
	if game == nil || game.ID == "" {
		return fmt.Errorf("jogo inválido para atualizar")
	}

	genresJSON, _ := json.Marshal(game.Genres)
	devsJSON, _ := json.Marshal(game.Developers)
	pubsJSON, _ := json.Marshal(game.Publishers)
	tagsJSON, _ := json.Marshal(game.Tags)
	categoriesJSON, _ := json.Marshal(game.Categories)
	featuresJSON, _ := json.Marshal(game.Features)
	platformsJSON, _ := json.Marshal(game.Platforms)

	result, err := db.conn.ExecContext(ctx, `
UPDATE games SET
	name = ?,
	sorting_name = ?,
	description = ?,
	release_date = ?,
	release_year = ?,
	version = ?,
	user_score = ?,
	critic_score = ?,
	community_score = ?,
	genres_json = ?,
	developers_json = ?,
	publishers_json = ?,
	tags_json = ?,
	categories_json = ?,
	features_json = ?,
	platforms_json = ?,
	trailer_url = ?,
	modified = CURRENT_TIMESTAMP
WHERE id = ?;`,
		game.Name,
		game.SortingName,
		game.Description,
		game.ReleaseDate,
		game.ReleaseYear,
		game.Version,
		game.UserScore,
		game.CriticScore,
		game.CommunityScore,
		string(genresJSON),
		string(devsJSON),
		string(pubsJSON),
		string(tagsJSON),
		string(categoriesJSON),
		string(featuresJSON),
		string(platformsJSON),
		game.TrailerURL,
		game.ID,
	)
	if err != nil {
		return fmt.Errorf("atualizar propriedades de %s: %w", game.Name, err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrGameNotFound
	}
	return nil
}

// ClearLibrary remove todos os jogos importados e os metadados de loja,
// mantendo as sessões e credenciais salvas. Usado pelo botão Limpar biblioteca.
func (db *DB) ClearLibrary(ctx context.Context) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM games;`); err != nil {
		return fmt.Errorf("limpar jogos: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM store_game_metadata;`); err != nil {
		return fmt.Errorf("limpar metadados de loja: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key LIKE 'launch.options.%';`); err != nil {
		return fmt.Errorf("limpar opções de inicialização: %w", err)
	}
	return tx.Commit()
}

// SetGameArtwork updates one explicitly selected artwork field.
func (db *DB) SetGameArtwork(ctx context.Context, id, field, image string) error {
	var column string
	switch field {
	case "icon":
		column = "icon"
	case "cover":
		column = "cover_image"
	case "background":
		column = "background_image"
	case "logo":
		column = "logo_image"
	default:
		return fmt.Errorf("campo de imagem desconhecido")
	}
	result, err := db.conn.ExecContext(ctx, "UPDATE games SET "+column+" = ?, modified = CURRENT_TIMESTAMP WHERE id = ?", image, id)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		return ErrGameNotFound
	}
	return nil
}
