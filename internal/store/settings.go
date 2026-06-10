package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// Configurações chave/valor. Hoje guarda só o hash do PIN da Mesa.

const settingPINHash = "mesa_pin_hash"

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM setting WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO setting(key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// hashPIN: o PIN não é guardado em texto puro. Não é cripto forte (PIN curto), mas
// evita expor o valor a quem só der uma olhada no banco. O limite de segurança real
// continua sendo o acesso ao arquivo .db (offline, na máquina da Mesa).
func hashPIN(pin string) string {
	sum := sha256.Sum256([]byte("votacao-ipb:" + pin))
	return hex.EncodeToString(sum[:])
}

// SetPIN define (ou troca) o PIN da Mesa.
func (s *Store) SetPIN(ctx context.Context, pin string) error {
	return s.SetSetting(ctx, settingPINHash, hashPIN(pin))
}

// PINHash devolve o hash do PIN salvo ("" se ainda não definido).
func (s *Store) PINHash(ctx context.Context) (string, error) {
	return s.GetSetting(ctx, settingPINHash)
}

// CheckPIN confere um PIN digitado contra o salvo.
func (s *Store) CheckPIN(ctx context.Context, pin string) (bool, error) {
	h, err := s.PINHash(ctx)
	if err != nil {
		return false, err
	}
	return h != "" && h == hashPIN(pin), nil
}
