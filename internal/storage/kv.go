package storage

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("chave não encontrada")

// KV guarda valores pequenos por chave. Implementações cifram o conteúdo.
type KV interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	// Delete não falha se a chave não existir.
	Delete(ctx context.Context, key string) error
}

const (
	KeyGoogleToken    = "google_token"
	KeyGoogleCalendar = "google_calendar_id"
	KeyPreferences    = "preferences"
	KeyMoodle         = "moodle_account"
	KeySyncState      = "sync_state"
	KeyPendingSeen    = "pending_seen"

	// Chaves globais (fora dos namespaces de usuário).
	KeySessions = "app_sessions"
	KeyUsers    = "users"
)

// UserKeys são todas as chaves do espaço de um usuário. Excluir a conta
// apaga cada uma delas; uma chave nova por usuário precisa entrar aqui.
var UserKeys = []string{KeyGoogleToken, KeyGoogleCalendar, KeyPreferences, KeyMoodle, KeySyncState, KeyPendingSeen}

// Namespace isola as chaves de um usuário dentro do mesmo KV.
func Namespace(kv KV, prefix string) KV { return namespaced{kv: kv, prefix: prefix} }

type namespaced struct {
	kv     KV
	prefix string
}

func (n namespaced) Get(ctx context.Context, key string) ([]byte, error) {
	return n.kv.Get(ctx, n.prefix+key)
}

func (n namespaced) Put(ctx context.Context, key string, value []byte) error {
	return n.kv.Put(ctx, n.prefix+key, value)
}

func (n namespaced) Delete(ctx context.Context, key string) error {
	return n.kv.Delete(ctx, n.prefix+key)
}
