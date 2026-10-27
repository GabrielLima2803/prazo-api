package storage

import (
	"context"
	"errors"
	"testing"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}
func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m memKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

func TestNamespaceIsolates(t *testing.T) {
	ctx := context.Background()
	root := memKV{}
	a, b := Namespace(root, "ua_"), Namespace(root, "ub_")
	a.Put(ctx, KeyPreferences, []byte("A"))
	if _, err := b.Get(ctx, KeyPreferences); !errors.Is(err, ErrNotFound) {
		t.Fatal("um usuário enxergou a chave do outro")
	}
	if string(root["ua_preferences"]) != "A" {
		t.Fatalf("root = %v", root)
	}
	a.Delete(ctx, KeyPreferences)
	if len(root) != 0 {
		t.Fatal("Delete não respeitou o namespace")
	}
}
