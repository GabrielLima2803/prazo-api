package tests

import (
	"github.com/GabrielLima2803/prazo-sync/internal/api"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeleteAccount(t *testing.T) {
	h := newHarness(t)
	ana := h.login(t, "ana")
	bia := h.login(t, "bia")
	host := strings.TrimPrefix(h.moodle.URL, "https://")
	anaID := tenant.IDFor(host, 42)

	if code := h.call(t, ana.Token, http.MethodPut, "/api/v1/preferences", `{"hiddenCourses":["POO"]}`, nil); code != http.StatusOK {
		t.Fatalf("preferências: %d", code)
	}
	h.root.Put(t.Context(), "u"+anaID+"_"+storage.KeyGoogleToken, []byte("x"))

	if code := h.call(t, ana.Token, http.MethodDelete, "/api/v1/account", "", nil); code != http.StatusOK {
		t.Fatalf("excluir: %d", code)
	}
	if h.forgotten != 1 {
		t.Errorf("o acesso ao Google deve ser revogado (%d)", h.forgotten)
	}
	if code := h.call(t, ana.Token, http.MethodGet, "/api/v1/status", "", nil); code != http.StatusUnauthorized {
		t.Errorf("a sessão deve acabar junto com a conta: %d", code)
	}

	h.root.mu.Lock()
	for key := range h.root.m {
		if strings.HasPrefix(key, "u"+anaID+"_") {
			t.Errorf("sobrou %s", key)
		}
	}
	users := string(h.root.m[storage.KeyUsers])
	h.root.mu.Unlock()
	if strings.Contains(users, anaID) {
		t.Error("a usuária continua na lista")
	}

	if code := h.call(t, bia.Token, http.MethodGet, "/api/v1/preferences", "", nil); code != http.StatusOK {
		t.Errorf("outra usuária foi afetada: %d", code)
	}
}

func TestExportHasNoSecrets(t *testing.T) {
	h := newHarness(t)
	ana := h.login(t, "ana")
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/api/v1/account/export", nil)
	req.Header.Set("Authorization", "Bearer "+ana.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"fullName":"ana"`) {
		t.Fatalf("export: %d %s", resp.StatusCode, body)
	}
	for _, secret := range []string{"ws-ana", "exp-ana", "authtoken", "certa"} {
		if strings.Contains(string(body), secret) {
			t.Errorf("a exportação vazou %q", secret)
		}
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Error("a exportação deve vir como arquivo")
	}
}

func TestLoginLimitPerAccount(t *testing.T) {
	h := newHarness(t)
	wrong := `{"baseUrl":"` + h.moodle.URL + `","username":"ana","password":"errada"}`
	for i := range api.LoginAttemptsPerAccount {
		if code := h.call(t, "", http.MethodPost, "/auth/login", wrong, nil); code != http.StatusUnprocessableEntity {
			t.Fatalf("tentativa %d: %d", i+1, code)
		}
	}
	if code := h.call(t, "", http.MethodPost, "/auth/login", wrong, nil); code != http.StatusTooManyRequests {
		t.Fatalf("depois do limite: %d", code)
	}
	// O limite é por conta: outra pessoa continua entrando.
	h.login(t, "bia")
}

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, quero %q", header, got, want)
		}
	}
}

func TestCORSOnlyForConfiguredOrigins(t *testing.T) {
	h := newHarness(t)
	for origin, allowed := range map[string]bool{"http://localhost:8081": true, "https://malicioso.example": false} {
		req, _ := http.NewRequest(http.MethodOptions, h.srv.URL+"/api/v1/status", nil)
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		got := resp.Header.Get("Access-Control-Allow-Origin")
		if allowed && got != origin || !allowed && got != "" {
			t.Errorf("%s: Allow-Origin = %q", origin, got)
		}
	}
}

func TestPrivacyPage(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.srv.URL + "/privacidade")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{"privacidade@example.com", "Fulano", "Uso Limitado", "Excluir conta"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("a página não menciona %q", want)
		}
	}
}

func TestInternalErrorsAreGeneric(t *testing.T) {
	h := newHarness(t)
	ana := h.login(t, "ana")
	host := strings.TrimPrefix(h.moodle.URL, "https://")
	// Preferências corrompidas no disco viram 500 sem detalhes.
	h.root.Put(t.Context(), "u"+tenant.IDFor(host, 42)+"_"+storage.KeyPreferences, []byte("{corrompido"))
	var body api.ErrorBody
	if code := h.call(t, ana.Token, http.MethodGet, "/api/v1/preferences", "", &body); code != http.StatusInternalServerError {
		t.Fatalf("status %d", code)
	}
	if body.Code != "internal" || strings.Contains(body.Error, "corrompido") || strings.Contains(body.Error, "invalid") {
		t.Errorf("o erro vazou detalhes: %+v", body)
	}
}
