package models

import "time"

// Game representa a entidade de um jogo no Playnux, espelhando todos os campos
// ricos do Playnite (Game.cs e GameField enum) adaptados para Go e SQLite.
type Game struct {
	ID          string `json:"id"`           // UUID
	GameID      string `json:"game_id"`      // ID único na loja (ex: AppID na Steam, slug no GOG)
	PluginID    string `json:"plugin_id"`    // ID do plugin/provedor da loja
	Source      string `json:"source"`       // Steam, GOG, Epic, Ubisoft, EA, etc.
	Name        string `json:"name"`         // Nome oficial do jogo
	SortingName string `json:"sorting_name"` // Nome para ordenação alfabética

	// Imagens e Mídia
	Icon            string `json:"icon"`             // URL ou Path local do ícone
	CoverImage      string `json:"cover_image"`      // URL ou Path da capa vertical (WebP)
	BackgroundImage string `json:"background_image"` // URL ou Path do background horizontal
	LogoImage       string `json:"logo_image"`       // Logo transparente (SteamGridDB)
	TrailerURL      string `json:"trailer_url"`      // URL do trailer oficial (YouTube)

	// Estado de Instalação e Execução
	IsInstalled      bool     `json:"is_installed"`      // true se está no disco, false se está apenas na conta digital
	InstallDirectory string   `json:"install_directory"` // Caminho no disco (se instalado)
	InstallSize      uint64   `json:"install_size"`      // Tamanho em bytes
	Executable       string   `json:"executable"`        // Binário/comando local (Linux, Flatpak e emuladores)
	Arguments        []string `json:"arguments"`         // Argumentos usados junto do Executable

	// Estatísticas de Gameplay
	Playtime     uint64     `json:"playtime"`      // Tempo jogado em segundos
	PlayCount    int        `json:"play_count"`    // Quantidade de vezes iniciado
	LastActivity *time.Time `json:"last_activity"` // Última vez que jogou

	// Metadados Ricos
	Description string     `json:"description"`  // Descrição completa (HTML ou Markdown)
	ReleaseDate *time.Time `json:"release_date"` // Data de lançamento oficial
	ReleaseYear int        `json:"release_year"` // Ano de lançamento
	Version     string     `json:"version"`      // Versão do jogo

	// Avaliações
	UserScore      *int `json:"user_score"`      // Avaliação do usuário (0 a 100)
	CriticScore    *int `json:"critic_score"`    // Avaliação da crítica (Metacritic/OpenCritic)
	CommunityScore *int `json:"community_score"` // Avaliação da comunidade da loja

	// Flags de Organização
	Hidden       bool `json:"hidden"`         // Ocultar da visualização padrão
	Favorite     bool `json:"favorite"`       // Marcado como favorito
	IsCustomGame bool `json:"is_custom_game"` // Jogo adicionado manualmente

	// Relacionamentos N:N (Armazenados como JSON ou tabelas de junção)
	Genres     []string `json:"genres"`     // Ação, RPG, Aventura, etc.
	Developers []string `json:"developers"` // Empresas desenvolvedoras
	Publishers []string `json:"publishers"` // Editoras / Publishers
	Tags       []string `json:"tags"`       // Tags comunitárias
	Categories []string `json:"categories"` // Categorias do usuário
	Features   []string `json:"features"`   // Single-player, Co-op, Suporte a Controle, etc.
	Platforms  []string `json:"platforms"`  // PC (Linux), PC (Windows), etc.

	// Timestamps do Banco
	Added    time.Time `json:"added"`    // Data em que entrou na biblioteca
	Modified time.Time `json:"modified"` // Última alteração de metadados

	// Variants lista as outras cópias do mesmo jogo quando a opção de unificar
	// títulos está ativa. Não é persistido: é montado pelo Core ao listar.
	Variants []GameVariant `json:"variants,omitempty"`
}

// GameVariant descreve uma cópia do jogo em outra loja/fonte. A UI usa os IDs
// para escolher de qual origem vêm as imagens em destaque.
type GameVariant struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Source          string `json:"source"`
	PluginID        string `json:"plugin_id"`
	Icon            string `json:"icon"`
	CoverImage      string `json:"cover_image"`
	BackgroundImage string `json:"background_image"`
	LogoImage       string `json:"logo_image"`
	TrailerURL      string `json:"trailer_url"`
	IsInstalled     bool   `json:"is_installed"`
}
