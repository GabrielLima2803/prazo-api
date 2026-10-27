package tests

import (
	"github.com/GabrielLima2803/prazo-sync/internal/api"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

func TestAuth(t *testing.T) {
	h := newHarness(t)
	for _, tok := range []string{"", "errado"} {
		if code := h.call(t, tok, http.MethodGet, "/api/v1/status", "", nil); code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", tok, code)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", adminToken) // sem "Bearer "
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sem Bearer: %d", resp.StatusCode)
	}
	if code := h.call(t, adminToken, http.MethodGet, "/api/v1/status", "", nil); code != http.StatusOK {
		t.Errorf("admin: %d", code)
	}
	resp, _ = http.Get(h.srv.URL + "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz = %d", resp.StatusCode)
	}
}

func TestUsersAreIsolated(t *testing.T) {
	h := newHarness(t)
	ana, bia := h.login(t, "ana"), h.login(t, "bia")
	if ana.Moodle.FullName != "ana" || bia.Moodle.FullName != "bia" {
		t.Fatalf("contas = %+v / %+v", ana.Moodle, bia.Moodle)
	}

	h.call(t, ana.Token, http.MethodPut, "/api/v1/preferences", `{"hiddenCourses":["POO"]}`, nil)
	var pa, pb sync.Preferences
	h.call(t, ana.Token, http.MethodGet, "/api/v1/preferences", "", &pa)
	h.call(t, bia.Token, http.MethodGet, "/api/v1/preferences", "", &pb)
	if len(pa.HiddenCourses) != 1 || len(pb.HiddenCourses) != 0 {
		t.Fatalf("preferências vazaram: ana=%+v bia=%+v", pa, pb)
	}

	var la, lb api.DeadlineList
	h.call(t, ana.Token, http.MethodGet, "/api/v1/deadlines", "", &la)
	h.call(t, bia.Token, http.MethodGet, "/api/v1/deadlines", "", &lb)
	if len(la.Deadlines) != 1 || len(lb.Deadlines) != 1 || la.Deadlines[0].ID == lb.Deadlines[0].ID {
		t.Fatalf("prazos: ana=%+v bia=%+v", la.Deadlines, lb.Deadlines)
	}
	if !la.Deadlines[0].Hidden || lb.Deadlines[0].Hidden {
		t.Fatal("a preferência da ana afetou a bia")
	}

	var run api.Run
	if code := h.call(t, bia.Token, http.MethodPost, "/api/v1/sync", "", &run); code != http.StatusOK && code != http.StatusConflict {
		t.Fatalf("sync: %d", code)
	}

	h.call(t, ana.Token, http.MethodPost, "/api/v1/auth/logout", "", nil)
	if code := h.call(t, ana.Token, http.MethodGet, "/api/v1/status", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("sessão da ana depois do logout: %d", code)
	}
	if code := h.call(t, bia.Token, http.MethodGet, "/api/v1/status", "", nil); code != http.StatusOK {
		t.Fatalf("o logout da ana derrubou a bia: %d", code)
	}

	// Entrar de novo reencontra o mesmo espaço.
	again := h.login(t, "ana")
	h.call(t, again.Token, http.MethodGet, "/api/v1/preferences", "", &pa)
	if len(pa.HiddenCourses) != 1 {
		t.Fatal("as preferências da ana se perderam ao entrar de novo")
	}
}

func TestLoginErrors(t *testing.T) {
	h := newHarness(t)
	var e api.ErrorBody
	if code := h.call(t, "", http.MethodPost, "/auth/login", fmt.Sprintf(`{"baseUrl":%q,"username":"ana","password":"errada"}`, h.moodle.URL), &e); code != http.StatusUnprocessableEntity || e.Code != "invalid_login" {
		t.Errorf("senha errada: %d %+v", code, e)
	}
	if code := h.call(t, "", http.MethodPost, "/auth/login", `{"baseUrl":"https://outro.example","username":"a","password":"b"}`, &e); code != http.StatusForbidden || e.Code != "host_not_allowed" {
		t.Errorf("host fora da lista: %d %+v", code, e)
	}
	if code := h.call(t, "", http.MethodPost, "/auth/ical", `{"icalUrl":"ftp://x"}`, &e); code != http.StatusUnprocessableEntity || e.Code != "invalid_url" {
		t.Errorf("ftp: %d %+v", code, e)
	}
	if strings.Contains(e.Error, "ftp://x") {
		t.Error("a mensagem não pode ecoar a URL")
	}
}

func TestDeadlinesWithoutMoodle(t *testing.T) {
	h := newHarness(t)
	var e api.ErrorBody
	if code := h.call(t, adminToken, http.MethodGet, "/api/v1/deadlines", "", &e); code != http.StatusConflict || e.Code != "moodle_not_connected" {
		t.Fatalf("deadlines sem Moodle: %d %+v", code, e)
	}
}

func TestInvalidPreferences(t *testing.T) {
	h := newHarness(t)
	if code := h.call(t, adminToken, http.MethodPut, "/api/v1/preferences", `{"courseColors":{"POO":"99"}}`, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("cor inválida: %d", code)
	}
	if code := h.call(t, adminToken, http.MethodPut, "/api/v1/preferences", `{"campoDesconhecido":1}`, nil); code != http.StatusBadRequest {
		t.Errorf("campo desconhecido: %d", code)
	}
}

func TestMeta(t *testing.T) {
	h := newHarness(t)
	var m api.Meta
	h.call(t, adminToken, http.MethodGet, "/api/v1/meta", "", &m)
	if len(m.Palette) != 11 || m.Palette[0].ID != "1" || len(m.Types) != len(domain.DeadlineTypes) {
		t.Fatalf("meta = %+v", m)
	}
}

func TestGoogleAppAuth(t *testing.T) {
	h := newHarness(t)
	ana := h.login(t, "ana")
	var auth api.GoogleAuthResponse
	if code := h.call(t, ana.Token, http.MethodPost, "/api/v1/google/auth", `{"returnTo":"exp://192.168.0.14:8081/--/google"}`, &auth); code != http.StatusOK || auth.AuthURL == "" {
		t.Fatalf("start: %d %+v", code, auth)
	}
	if h.google.startedBy != tenant.IDFor(strings.ToLower(strings.TrimPrefix(h.moodle.URL, "https://")), 42) {
		t.Fatalf("autorização começou para %q", h.google.startedBy)
	}

	resp, err := http.Get(h.srv.URL + "/oauth/google/callback?" + url.Values{"state": {"s1"}, "code": {"c1"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `href="exp://192.168.0.14:8081/--/google?google=ok"`) {
		t.Fatalf("callback: %d\n%s", resp.StatusCode, body)
	}
	resp, _ = http.Get(h.srv.URL + "/oauth/google/callback?state=forjado&code=c1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("state forjado: %d", resp.StatusCode)
	}
}
