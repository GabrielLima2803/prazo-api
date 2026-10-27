package session

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}
func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m memKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

func TestIssueUserRevoke(t *testing.T) {
	ctx := context.Background()
	kv := memKV{}
	s := New(kv)
	tok, err := s.Issue(ctx, "ana")
	if err != nil || len(tok) < 40 {
		t.Fatalf("token = %q, %v", tok, err)
	}
	if bytes.Contains(kv[store.KeySessions], []byte(tok)) {
		t.Fatal("token guardado em texto plano")
	}
	if u, ok := s.User(ctx, tok); !ok || u != "ana" {
		t.Fatalf("User = %q, %v", u, ok)
	}
	if _, ok := s.User(ctx, "outro"); ok {
		t.Fatal("token desconhecido aceito")
	}
	if err := s.Revoke(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.User(ctx, tok); ok {
		t.Fatal("sessão revogada ainda vale")
	}
}

func TestLimitIsPerUser(t *testing.T) {
	ctx := context.Background()
	s := New(memKV{})
	first, _ := s.Issue(ctx, "ana")
	bia, _ := s.Issue(ctx, "bia")
	for range MaxSessions {
		s.Issue(ctx, "ana")
	}
	if _, ok := s.User(ctx, first); ok {
		t.Fatal("a sessão mais antiga da ana deveria ter saído")
	}
	if u, ok := s.User(ctx, bia); !ok || u != "bia" {
		t.Fatal("as sessões da ana derrubaram a da bia")
	}
	s.RevokeUser(ctx, "ana")
	if _, ok := s.User(ctx, bia); !ok {
		t.Fatal("RevokeUser afetou outro usuário")
	}
}

func TestSessionsExpire(t *testing.T) {
	kv := memKV{}
	s := New(kv)
	now := time.Now()
	s.now = func() time.Time { return now }
	token, err := s.Issue(context.Background(), "ana")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(MaxAge + time.Hour)
	if _, ok := s.User(context.Background(), token); ok {
		t.Fatal("sessão vencida ainda vale")
	}
	// A próxima emissão descarta as vencidas.
	if _, err := s.Issue(context.Background(), "bia"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.load(context.Background())
	if len(list) != 1 {
		t.Errorf("sessões guardadas: %d", len(list))
	}
}
