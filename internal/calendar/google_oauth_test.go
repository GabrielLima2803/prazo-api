package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// tokenServer imita o endpoint de token do Google e confere o PKCE.
func tokenServer(t *testing.T, refresh, scope string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "abc" || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": scope,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// authorize roda o fluxo e simula o navegador seguindo o redirect com os
// parâmetros que mutate devolver.
func authorize(t *testing.T, tokenURL string, mutate func(q url.Values)) (*oauth2.Token, error) {
	t.Helper()
	cfg := &oauth2.Config{ClientID: "id", ClientSecret: "s", Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: tokenURL}, Scopes: []string{GoogleScope}}
	return Authorize(context.Background(), cfg, "127.0.0.1:0", func(authURL string) {
		u, _ := url.Parse(authURL)
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" || q.Get("prompt") != "consent" {
			t.Errorf("URL de autorização sem PKCE/offline/consent: %s", authURL)
		}
		if !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") {
			t.Errorf("redirect_uri = %s", q.Get("redirect_uri"))
		}
		cb := url.Values{"state": {q.Get("state")}, "code": {"abc"}}
		mutate(cb)
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?" + cb.Encode())
			if err == nil {
				resp.Body.Close()
			}
		}()
	})
}

func TestAuthorize(t *testing.T) {
	srv := tokenServer(t, "1//refresh", GoogleScope)
	tok, err := authorize(t, srv.URL, func(url.Values) {})
	if err != nil || tok.RefreshToken != "1//refresh" {
		t.Fatalf("tok = %+v, err = %v", tok, err)
	}
}

func TestAuthorizeFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		refresh, scope string
		mutate         func(url.Values)
		want           error
	}{
		"state forjado":   {"r", GoogleScope, func(q url.Values) { q.Set("state", "x") }, ErrStateMismatch},
		"sem refresh":     {"", GoogleScope, func(url.Values) {}, ErrNoRefreshToken},
		"escopo negado":   {"r", "openid", func(url.Values) {}, ErrScopeDenied},
		"usuário recusou": {"r", GoogleScope, func(q url.Values) { q.Del("code"); q.Set("error", "access_denied") }, nil},
	} {
		srv := tokenServer(t, tc.refresh, tc.scope)
		_, err := authorize(t, srv.URL, tc.mutate)
		if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s: err = %v; quer %v", name, err, tc.want)
		}
	}
}
