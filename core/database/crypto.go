package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
)

const keySize = 32

// loadOrCreateKey carrega a chave local do banco (0600) ou gera uma nova.
// A chave fica ao lado do arquivo SQLite, com permissões restritas ao usuário.
func loadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != keySize {
			return nil, fmt.Errorf("chave de sessão inválida em %s", path)
		}
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("erro ao gerar chave de sessão: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("erro ao salvar chave de sessão: %w", err)
	}
	return key, nil
}

// seal cifra o payload com AES-256-GCM e prefixa o nonce.
func (db *DB) seal(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(db.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (db *DB) openSealed(blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(db.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, fmt.Errorf("payload de sessão truncado")
	}

	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
