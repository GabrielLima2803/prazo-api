// Package store define onde o Prazo guarda estado. Até a Fase 3 (SQLite), o
// estado é um conjunto pequeno de chaves em arquivos cifrados (filekv).
package store

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("chave não encontrada")

// KV guarda valores pequenos por chave. Implementações cifram o conteúdo.
type KV interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
}

const (
	KeyGoogleToken    = "google_token"
	KeyGoogleCalendar = "google_calendar_id"
	KeyPreferences    = "preferences"
)
