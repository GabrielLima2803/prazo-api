package tenant

import (
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	gosync "sync"
	"time"
)

// MaxSessions limita os aparelhos conectados por usuário; a sessão mais
// antiga sai primeiro.
const MaxSessions = 10

// SessionMaxAge é a validade de uma sessão: depois disso o app pede login de novo.
// Limita o estrago de um aparelho perdido e evita guardar sessões esquecidas.
const SessionMaxAge = 180 * 24 * time.Hour

type Session struct {
	Hash      string    `json:"hash"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
}

type Sessions struct {
	kv  storage.KV
	now func() time.Time
	mu  gosync.Mutex
}

func NewSessions(kv storage.KV) *Sessions { return &Sessions{kv: kv, now: time.Now} }

func (s *Sessions) Issue(ctx context.Context, userID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load(ctx)
	if err != nil {
		return "", err
	}
	now := s.now()
	list = slices.DeleteFunc(list, func(x Session) bool { return s.expired(x, now) })
	token := rand.Text() + rand.Text()
	list = append(list, Session{Hash: hash(token), UserID: userID, CreatedAt: s.now().UTC()})
	// Mantém as MaxSessions mais recentes do usuário (a lista está em ordem).
	var mine int
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].UserID != userID {
			continue
		}
		if mine++; mine > MaxSessions {
			list = slices.Delete(list, i, i+1)
		}
	}
	return token, s.save(ctx, list)
}

// User devolve o usuário dono da sessão; ok=false para token desconhecido.
func (s *Sessions) User(ctx context.Context, token string) (string, bool) {
	if token == "" {
		return "", false
	}
	list, err := s.load(ctx)
	if err != nil {
		return "", false
	}
	h := []byte(hash(token))
	now := s.now()
	for _, sess := range list {
		if sess.UserID != "" && !s.expired(sess, now) && subtle.ConstantTimeCompare([]byte(sess.Hash), h) == 1 {
			return sess.UserID, true
		}
	}
	return "", false
}

func (s *Sessions) Revoke(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load(ctx)
	if err != nil {
		return err
	}
	h := hash(token)
	return s.save(ctx, slices.DeleteFunc(list, func(x Session) bool { return x.Hash == h }))
}

// RevokeUser encerra todos os aparelhos de um usuário.
func (s *Sessions) RevokeUser(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load(ctx)
	if err != nil {
		return err
	}
	return s.save(ctx, slices.DeleteFunc(list, func(x Session) bool { return x.UserID == userID }))
}

func (s *Sessions) load(ctx context.Context) ([]Session, error) {
	b, err := s.kv.Get(ctx, storage.KeySessions)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Session
	return list, json.Unmarshal(b, &list)
}

func (s *Sessions) save(ctx context.Context, list []Session) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.kv.Put(ctx, storage.KeySessions, b)
}

func (s *Sessions) expired(x Session, now time.Time) bool {
	return now.Sub(x.CreatedAt) > SessionMaxAge
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
