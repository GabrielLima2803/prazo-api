package calendar

import (
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"context"
	"errors"
	"net/url"
	"os"
	"testing"

	"golang.org/x/oauth2"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, storage.ErrNotFound
}
func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m memKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

func newAppAuth(t *testing.T, kv storage.KV, refresh string) *AppAuth {
	srv := tokenServer(t, refresh, GoogleScope)
	cfg := &oauth2.Config{ClientID: "web-id", ClientSecret: "s", Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: srv.URL}, Scopes: []string{GoogleScope}}
	return NewAppAuth(cfg, "https://prazo.example.ts.net/", func(string) storage.KV { return kv })
}

func TestAppAuthFlow(t *testing.T) {
	kv := memKV{}
	a := newAppAuth(t, kv, "1//web")

	authURL, err := a.Start("ana", "exp://192.168.0.14:8081/--/google")
	if err != nil {
		t.Fatal(err)
	}
	q := mustQuery(t, authURL)
	if q.Get("redirect_uri") != "https://prazo.example.ts.net/oauth/google/callback" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("auth URL = %s", authURL)
	}

	user, returnTo, err := a.Finish(context.Background(), q.Get("state"), "abc")
	if err != nil || user != "ana" || returnTo != "exp://192.168.0.14:8081/--/google" {
		t.Fatalf("Finish = %q %q, %v", user, returnTo, err)
	}
	st, err := LoadToken(context.Background(), kv)
	if err != nil || st.Client != ClientWeb || st.Token.RefreshToken != "1//web" {
		t.Fatalf("token salvo = %+v, %v", st, err)
	}

	if _, _, err := a.Finish(context.Background(), q.Get("state"), "abc"); !errors.Is(err, ErrUnknownState) {
		t.Fatalf("state reutilizado: %v", err)
	}
}

func TestAppAuthRejects(t *testing.T) {
	a := newAppAuth(t, memKV{}, "r")
	for _, bad := range []string{"https://evil.example", "http://x", "javascript://alert(1)", "sem-esquema"} {
		if _, err := a.Start("ana", bad); !errors.Is(err, ErrInvalidReturnTo) {
			t.Errorf("returnTo %q aceito", bad)
		}
	}
	if _, _, err := a.Finish(context.Background(), "forjado", "abc"); !errors.Is(err, ErrUnknownState) {
		t.Errorf("state forjado: %v", err)
	}
}

func TestLoadLegacyToken(t *testing.T) {
	kv := memKV{storage.KeyGoogleToken: []byte(`{"access_token":"a","refresh_token":"1//antigo","token_type":"Bearer"}`)}
	st, err := LoadToken(context.Background(), kv)
	if err != nil || st.Client != ClientInstalled || st.Token.RefreshToken != "1//antigo" {
		t.Fatalf("legado = %+v, %v", st, err)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestLoadWebConfig(t *testing.T) {
	dir := t.TempDir()
	web := dir + "/web.json"
	os.WriteFile(web, []byte(`{"web":{"client_id":"id.apps.googleusercontent.com","client_secret":"s","auth_uri":"x","token_uri":"y"}}`), 0o600)
	cfg, err := LoadWebOAuthConfig(web)
	if err != nil || cfg.ClientID != "id.apps.googleusercontent.com" || cfg.Endpoint.TokenURL == "" {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}
	installed := dir + "/installed.json"
	os.WriteFile(installed, []byte(`{"installed":{"client_id":"id","client_secret":"s"}}`), 0o600)
	if _, err := LoadWebOAuthConfig(installed); err == nil {
		t.Error("aceitou credencial de computador como web")
	}
}
