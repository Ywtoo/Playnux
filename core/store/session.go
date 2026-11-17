package store

import (
	"sort"
	"strings"

	"github.com/playnux/core/models"
)

// SessionFromAuth converte credenciais em uma sessão persistível.
func SessionFromAuth(pluginID string, auth StoreAuth) *models.Session {
	return &models.Session{
		PluginID:     pluginID,
		AccountID:    auth.AccountID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		ExpiresAt:    auth.ExpiresAt,
		Cookies:      auth.Cookies,
		APIKey:       auth.APIKey,
	}
}

// AuthFromSession reconstrói as credenciais a partir de uma sessão salva.
func AuthFromSession(s *models.Session) StoreAuth {
	return StoreAuth{
		AccountID:    s.AccountID,
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		ExpiresAt:    s.ExpiresAt,
		Cookies:      s.Cookies,
		APIKey:       s.APIKey,
	}
}

// FormatCookies serializa cookies de forma determinística ("a=1; b=2").
func FormatCookies(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+cookies[name])
	}
	return strings.Join(parts, "; ")
}
