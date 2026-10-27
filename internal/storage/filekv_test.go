package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T, dir string) *FileKV {
	t.Helper()
	key, _ := ParseKey(GenerateKey())
	sealer, err := NewSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewFileKV(dir, sealer)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPutGet(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := newStore(t, dir)

	if _, err := s.Get(ctx, KeyGoogleToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get vazio: %v", err)
	}
	secret := []byte(`{"refresh_token":"1//segredo"}`)
	if err := s.Put(ctx, KeyGoogleToken, secret); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, KeyGoogleToken)
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
	if _, err := newStore(t, dir).Get(ctx, "a"); !errors.Is(err, ErrCiphertext) {
		t.Errorf("chave mestra diferente: %v", err)
	}
	if err := newStore(t, dir).Put(ctx, "../fora", nil); err == nil {
		t.Error("aceitou path traversal")
	}
}
