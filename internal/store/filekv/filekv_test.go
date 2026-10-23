package filekv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GabrielLima2803/prazo-sync/internal/crypto"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

func newStore(t *testing.T, dir string) *Store {
	t.Helper()
	key, _ := crypto.ParseKey(crypto.GenerateKey())
	sealer, err := crypto.NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, sealer)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPutGet(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := newStore(t, dir)

	if _, err := s.Get(ctx, store.KeyGoogleToken); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get vazio: %v", err)
	}
	secret := []byte(`{"refresh_token":"1//segredo"}`)
	if err := s.Put(ctx, store.KeyGoogleToken, secret); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, store.KeyGoogleToken)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Get = %q, %v", got, err)
	}

	path := filepath.Join(dir, "google_token.enc")
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("segredo")) {
		t.Fatal("segredo em texto plano no disco")
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("permissão = %v", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("sobraram arquivos temporários: %v", entries)
	}
}

func TestRejectsOtherKeyAndBadNames(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := newStore(t, dir).Put(ctx, "a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(t, dir).Get(ctx, "a"); !errors.Is(err, crypto.ErrCiphertext) {
		t.Errorf("chave mestra diferente: %v", err)
	}
	if err := newStore(t, dir).Put(ctx, "../fora", nil); err == nil {
		t.Error("aceitou path traversal")
	}
}
