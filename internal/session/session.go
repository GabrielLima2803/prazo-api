// Package session emite as sessões do app: depois de entrar com o Moodle, o
// app recebe um token aleatório. Só o hash SHA-256 fica guardado, então nem
// quem lê o disco consegue se passar pelo app.
package session

import (
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

	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

// MaxSessions limita os aparelhos conectados; a mais antiga sai primeiro.
const MaxSessions = 10

type Session struct {
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"createdAt"`
}

type Service struct {
	kv  store.KV
	now func() time.Time
	mu  gosync.Mutex
}

func New(kv store.KV) *Service { return &Service{kv: kv, now: time.Now} }

func (s *Service) Issue(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load(ctx)
	if err != nil {
		return "", err
	}
	token := rand.Text() + rand.Text()
	list = append(list, Session{Hash: hash(token), CreatedAt: s.now().UTC()})
	if len(list) > MaxSessions {
		list = list[len(list)-MaxSessions:]
	}
	return token, s.save(ctx, list)
}

func (s *Service) Valid(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	list, err := s.load(ctx)
	if err != nil {
		return false
	}
	h := []byte(hash(token))
	for _, sess := range list {
		if subtle.ConstantTimeCompare([]byte(sess.Hash), h) == 1 {
			return true
		}
	}
	return false
}

func (s *Service) Revoke(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load(ctx)
	if err != nil {
		return err
	}
	h := hash(token)
	return s.save(ctx, slices.DeleteFunc(list, func(x Session) bool { return x.Hash == h }))
}

// RevokeAll encerra todos os aparelhos (por exemplo, ao trocar de dono).
func (s *Service) RevokeAll(ctx context.Context) error {
	return s.kv.Delete(ctx, store.KeySessions)
}

func (s *Service) load(ctx context.Context) ([]Session, error) {
	b, err := s.kv.Get(ctx, store.KeySessions)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Session
	return list, json.Unmarshal(b, &list)
}

func (s *Service) save(ctx context.Context, list []Session) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.kv.Put(ctx, store.KeySessions, b)
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
