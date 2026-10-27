package tenant

import (
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"bytes"
	"context"
	"testing"
	"time"
)

func TestIssueUserRevoke(t *testing.T) {
	ctx := context.Background()
	kv := memKV{}
	s := NewSessions(kv)
	tok, err := s.Issue(ctx, "ana")
	if err != nil || len(tok) < 40 {
		t.Fatalf("token = %q, %v", tok, err)
	}
	if bytes.Contains(kv[storage.KeySessions], []byte(tok)) {
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
	s := NewSessions(memKV{})
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
	s := NewSessions(kv)
	now := time.Now()
	s.now = func() time.Time { return now }
	token, err := s.Issue(context.Background(), "ana")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(SessionMaxAge + time.Hour)
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
