package models

import "time"

// Session é a sessão digital persistida de uma loja. Sensível por natureza:
// o pacote database grava esse registro cifrado (AES-GCM) no SQLite.
type Session struct {
	PluginID     string    `json:"plugin_id"`
	AccountID    string    `json:"account_id,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Cookies      string    `json:"cookies,omitempty"`
	APIKey       string    `json:"api_key,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}
