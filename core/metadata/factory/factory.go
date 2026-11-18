// Package factory monta o serviço de metadados a partir das variáveis de
// ambiente, sem criar um ciclo entre o pacote base e os provedores.
package factory

import (
	"os"
	"strings"

	"github.com/playnux/core/metadata"
	"github.com/playnux/core/metadata/igdb"
	"github.com/playnux/core/metadata/steamgriddb"
)

// NewFromEnv ativa apenas os provedores cujas chaves estão presentes.
func NewFromEnv() *metadata.Service {
	return New("", "", "")
}

// New gives in-app credentials precedence over environment defaults.
func New(clientID, secret, gridKey string) *metadata.Service {
	service := &metadata.Service{}
	if clientID == "" || secret == "" {
		clientID = firstEnv("IGDB_CLIENT_ID", "TWITCH_CLIENT_ID")
		secret = firstEnv("IGDB_CLIENT_SECRET", "TWITCH_CLIENT_SECRET")
	}
	if gridKey == "" {
		gridKey = firstEnv("STEAMGRIDDB_API_KEY", "SGDB_API_KEY")
	}
	if clientID != "" && secret != "" {
		service.Info = igdb.NewIGDB(clientID, secret)
	}
	if gridKey != "" {
		service.Art = steamgriddb.NewSteamGridDB(gridKey)
	}
	return service
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
