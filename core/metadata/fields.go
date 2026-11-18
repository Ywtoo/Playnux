package metadata

import "github.com/playnux/core/models"

type Field struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
	present  func(*models.Game) bool
	restore  func(dst, src *models.Game)
}

var Fields = []Field{
	{"description", "Sinopse", "igdb", func(g *models.Game) bool { return g.Description != "" }, func(d, s *models.Game) { d.Description = s.Description }},
	{"release_date", "Data de lançamento", "igdb", func(g *models.Game) bool { return g.ReleaseDate != nil || g.ReleaseYear > 0 }, func(d, s *models.Game) { d.ReleaseDate, d.ReleaseYear = s.ReleaseDate, s.ReleaseYear }},
	{"genres", "Gêneros", "igdb", func(g *models.Game) bool { return len(g.Genres) > 0 }, func(d, s *models.Game) { d.Genres = s.Genres }},
	{"developers", "Desenvolvedores", "igdb", func(g *models.Game) bool { return len(g.Developers) > 0 }, func(d, s *models.Game) { d.Developers = s.Developers }},
	{"publishers", "Distribuidoras", "igdb", func(g *models.Game) bool { return len(g.Publishers) > 0 }, func(d, s *models.Game) { d.Publishers = s.Publishers }},
	{"critic_score", "Nota da crítica", "igdb", func(g *models.Game) bool { return g.CriticScore != nil }, func(d, s *models.Game) { d.CriticScore = s.CriticScore }},
	{"community_score", "Nota da comunidade", "igdb", func(g *models.Game) bool { return g.CommunityScore != nil }, func(d, s *models.Game) { d.CommunityScore = s.CommunityScore }},
	{"tags", "Tags", "igdb", func(g *models.Game) bool { return len(g.Tags) > 0 }, func(d, s *models.Game) { d.Tags = s.Tags }},
	{"features", "Modos de jogo e recursos", "igdb", func(g *models.Game) bool { return len(g.Features) > 0 }, func(d, s *models.Game) { d.Features = s.Features }},
	{"platforms", "Plataformas", "igdb", func(g *models.Game) bool { return len(g.Platforms) > 0 }, func(d, s *models.Game) { d.Platforms = s.Platforms }},
	{"icon", "Ícone", "steamgriddb", func(g *models.Game) bool { return g.Icon != "" }, func(d, s *models.Game) { d.Icon = s.Icon }},
	{"trailer", "Trailer", "igdb", func(g *models.Game) bool { return g.TrailerURL != "" }, func(d, s *models.Game) { d.TrailerURL = s.TrailerURL }},
	{"cover", "Capa", "steamgriddb", func(g *models.Game) bool { return g.CoverImage != "" }, func(d, s *models.Game) { d.CoverImage = s.CoverImage }},
	{"background", "Imagem de fundo", "steamgriddb", func(g *models.Game) bool { return g.BackgroundImage != "" }, func(d, s *models.Game) { d.BackgroundImage = s.BackgroundImage }},
	{"logo", "Logotipo", "steamgriddb", func(g *models.Game) bool { return g.LogoImage != "" }, func(d, s *models.Game) { d.LogoImage = s.LogoImage }},
}
