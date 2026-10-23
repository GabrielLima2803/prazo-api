package session

import (
	"bytes"
	"context"
	"testing"

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

func TestIssueValidRevoke(t *testing.T) {
	ctx := context.Background()
	kv := memKV{}
	s := New(kv)
	tok, err := s.Issue(ctx)
	if err != nil || len(tok) < 40 {
		t.Fatalf("token = %q, %v", tok, err)
	}
	if bytes.Contains(kv[store.KeySessions], []byte(tok)) {
		t.Fatal("token guardado em texto plano")
	}
	if !s.Valid(ctx, tok) || s.Valid(ctx, "outro") || s.Valid(ctx, "") {
		t.Fatal("Valid errado")
	}
	if err := s.Revoke(ctx, tok); err != nil || s.Valid(ctx, tok) {
		t.Fatalf("depois de Revoke: %v", err)
	}
}

func TestOldestSessionsDropped(t *testing.T) {
	ctx := context.Background()
	s := New(memKV{})
	first, _ := s.Issue(ctx)
	for range MaxSessions {
		s.Issue(ctx)
	}
	if s.Valid(ctx, first) {
		t.Fatal("a sessão mais antiga deveria ter saído")
	}
}
