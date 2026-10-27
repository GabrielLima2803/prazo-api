package google

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"

	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type mapKV map[string][]byte

func (m mapKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}
func (m mapKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m mapKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

func TestForget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wantErr error
	}{
		{"revogado", http.StatusOK, nil},
		{"já inválido", http.StatusBadRequest, nil},
		{"Google fora do ar", http.StatusServiceUnavailable, ErrRevoke},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				got = r.Form.Get("token")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			kv := mapKV{}
			SaveToken(context.Background(), kv, ClientWeb, &oauth2.Token{AccessToken: "a", RefreshToken: "r"})
			rv := &Revoker{endpoint: srv.URL, client: srv.Client()}

			err := rv.Forget(context.Background(), kv)
			if !errors.Is(err, tc.wantErr) && !(err == nil && tc.wantErr == nil) {
				t.Fatalf("err = %v, quero %v", err, tc.wantErr)
			}
			if got != "r" {
				t.Errorf("revogou %q, quero o refresh token", got)
			}
			if _, ok := kv[store.KeyGoogleToken]; ok {
				t.Error("o token local deve ser apagado mesmo se o Google falhar")
			}
		})
	}
}

func TestForgetWithoutToken(t *testing.T) {
	if err := NewRevoker(nil).Forget(context.Background(), mapKV{}); err != nil {
		t.Fatal(err)
	}
}
