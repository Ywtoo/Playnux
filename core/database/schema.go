package database

// SchemaSQL contém o DDL completo para o SQLite do Playnux.
// Já nasce completo, espelhando os campos reais do Playnite, com índices rápidos
// para busca instantânea e suporte a WAL (Write-Ahead Logging).
const SchemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

-- Tabela Principal de Jogos
CREATE TABLE IF NOT EXISTS games (
    id                  TEXT PRIMARY KEY,
    game_id             TEXT NOT NULL,
    plugin_id           TEXT NOT NULL,
    source              TEXT NOT NULL,
    name                TEXT NOT NULL,
    sorting_name        TEXT NOT NULL,
    
    icon                TEXT,
    cover_image         TEXT,
    background_image    TEXT,
    logo_image          TEXT,
    trailer_url         TEXT,
    
    is_installed        INTEGER NOT NULL DEFAULT 0,
    install_directory   TEXT,
    install_size        INTEGER DEFAULT 0,
    executable          TEXT,
    arguments_json      TEXT DEFAULT '[]',
    
    playtime            INTEGER DEFAULT 0,
    play_count          INTEGER DEFAULT 0,
    last_activity       DATETIME,
    
    description         TEXT,
    release_date        DATETIME,
    release_year        INTEGER,
    version             TEXT,
    
    user_score          INTEGER,
    critic_score        INTEGER,
    community_score     INTEGER,
    
    hidden              INTEGER NOT NULL DEFAULT 0,
    favorite            INTEGER NOT NULL DEFAULT 0,
    is_custom_game      INTEGER NOT NULL DEFAULT 0,
    
    genres_json         TEXT DEFAULT '[]',
    developers_json     TEXT DEFAULT '[]',
    publishers_json     TEXT DEFAULT '[]',
    tags_json           TEXT DEFAULT '[]',
    categories_json     TEXT DEFAULT '[]',
    features_json       TEXT DEFAULT '[]',
    platforms_json      TEXT DEFAULT '[]',
    
    added               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    modified            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(source, game_id)
);

-- Índices de Performance Máxima
CREATE INDEX IF NOT EXISTS idx_games_source ON games(source);
CREATE INDEX IF NOT EXISTS idx_games_installed ON games(is_installed);
CREATE INDEX IF NOT EXISTS idx_games_favorite ON games(favorite);
CREATE INDEX IF NOT EXISTS idx_games_hidden ON games(hidden);
CREATE INDEX IF NOT EXISTS idx_games_last_activity ON games(last_activity DESC);
CREATE INDEX IF NOT EXISTS idx_games_sorting_name ON games(sorting_name ASC);

-- Tabela Virtual para Busca Textual Instantânea (FTS5)
CREATE VIRTUAL TABLE IF NOT EXISTS games_fts USING fts5(
    id UNINDEXED,
    name,
    sorting_name,
    genres_json,
    developers_json,
    publishers_json,
    tags_json,
    content='games',
    content_rowid='rowid'
);

-- Triggers para manter o FTS5 sincronizado automaticamente
CREATE TRIGGER IF NOT EXISTS games_ai AFTER INSERT ON games BEGIN
  INSERT INTO games_fts(rowid, id, name, sorting_name, genres_json, developers_json, publishers_json, tags_json)
  VALUES (new.rowid, new.id, new.name, new.sorting_name, new.genres_json, new.developers_json, new.publishers_json, new.tags_json);
END;

CREATE TRIGGER IF NOT EXISTS games_ad AFTER DELETE ON games BEGIN
  INSERT INTO games_fts(games_fts, rowid, id, name, sorting_name, genres_json, developers_json, publishers_json, tags_json)
  VALUES('delete', old.rowid, old.id, old.name, old.sorting_name, old.genres_json, old.developers_json, old.publishers_json, old.tags_json);
END;

CREATE TRIGGER IF NOT EXISTS games_au AFTER UPDATE ON games BEGIN
  INSERT INTO games_fts(games_fts, rowid, id, name, sorting_name, genres_json, developers_json, publishers_json, tags_json)
  VALUES('delete', old.rowid, old.id, old.name, old.sorting_name, old.genres_json, old.developers_json, old.publishers_json, old.tags_json);
  INSERT INTO games_fts(rowid, id, name, sorting_name, genres_json, developers_json, publishers_json, tags_json)
  VALUES (new.rowid, new.id, new.name, new.sorting_name, new.genres_json, new.developers_json, new.publishers_json, new.tags_json);
END;

-- Sessões digitais das lojas (tokens/cookies cifrados com AES-GCM pelo Core)
CREATE TABLE IF NOT EXISTS store_sessions (
    plugin_id   TEXT PRIMARY KEY,
    payload     BLOB NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Preferências locais (pastas de jogos DRM-free e ROMs, etc.)
CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Emuladores e perfis cadastrados manualmente pelo usuário.
CREATE TABLE IF NOT EXISTS emulators (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    executable    TEXT NOT NULL DEFAULT '',
    flatpak_id    TEXT NOT NULL DEFAULT '',
    profiles_json TEXT NOT NULL DEFAULT '[]',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Metadados originais reportados pela loja.
CREATE TABLE IF NOT EXISTS store_game_metadata (
    plugin_id TEXT NOT NULL,
    game_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    PRIMARY KEY (plugin_id, game_id)
);
`
