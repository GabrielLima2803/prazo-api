package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/calendartest"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/session"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

const adminToken = "token-de-admin-com-tamanho-suficiente"

// memKV é seguro para uso concorrente: a sincronização em segundo plano
// escreve enquanto o teste lê.
type memKV struct {
	mu gosync.Mutex
	m  map[string][]byte
}

func newMemKV() *memKV { return &memKV{m: map[string][]byte{}} }

func (k *memKV) Get(_ context.Context, key string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[key]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}

func (k *memKV) Put(_ context.Context, key string, v []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[key] = v
	return nil
}

func (k *memKV) Delete(_ context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, key)
	return nil
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeMoodle tem dois alunos: ana (userid 42) e bia (userid 43).
func fakeMoodle(t *testing.T) *httptest.Server {
	t.Helper()
	users := map[string]int{"ana": 42, "bia": 43}
	due := time.Now().Add(72 * time.Hour).UTC().Format("20060102T150405Z")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch {
		case r.URL.Path == "/login/token.php":
			if _, ok := users[r.Form.Get("username")]; !ok || r.Form.Get("password") != "certa" {
				io.WriteString(w, `{"error":"x","errorcode":"invalidlogin"}`)
				return
			}
			io.WriteString(w, `{"token":"ws-`+r.Form.Get("username")+`"}`)
		case r.Form.Get("wsfunction") == "core_webservice_get_site_info":
			name := strings.TrimPrefix(r.Form.Get("wstoken"), "ws-")
			fmt.Fprintf(w, `{"sitename":"M","fullname":"%s","userid":%d,"functions":[{"name":"core_calendar_get_calendar_export_token"}]}`, name, users[name])
		case r.Form.Get("wsfunction") == "core_calendar_get_calendar_export_token":
			io.WriteString(w, `{"token":"exp-`+strings.TrimPrefix(r.Form.Get("wstoken"), "ws-")+`"}`)
		case r.Form.Get("wsfunction") == "core_calendar_get_action_events_by_timesort":
			io.WriteString(w, `{"events":[]}`)
		case r.URL.Path == "/calendar/export_execute.php":
			if !strings.HasPrefix(r.URL.Query().Get("authtoken"), "exp-") {
				io.WriteString(w, "Invalid authentication")
				return
			}
			uid := r.URL.Query().Get("userid")
			fmt.Fprintf(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:%s1@moodle\r\nSUMMARY:Lista %s é devido\r\nCATEGORIES:2026/2_CCT_CCI251-03A_POO0002\r\nDTSTART:%s\r\nDTEND:%s\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", uid, uid, due, due)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type harness struct {
	srv    *httptest.Server
	moodle *httptest.Server
	root   *memKV
	sinks  map[string]*calendartest.Fake
	google *fakeGoogleAuth
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{moodle: fakeMoodle(t), root: newMemKV(), sinks: map[string]*calendartest.Fake{}, google: &fakeGoogleAuth{}}
	client := httpx.New(httpx.Options{Transport: h.moodle.Client().Transport})
	host := strings.TrimPrefix(h.moodle.URL, "https://")

	manager := tenant.NewManager(h.root, func(u tenant.User, kv store.KV) *tenant.Tenant {
		accounts := account.NewService(kv, client, discard, "")
		ps := prefs.NewStore(kv)
		sink := calendartest.New()
		h.sinks[u.ID] = sink
		runner := sync.NewRunner(sync.NewService(accounts.Source(), domain.NormalizeOptions{}), func(ctx context.Context) (sync.Target, error) {
			p, err := ps.Load(ctx)
			return sync.Target{Sink: sink, State: kv, Calendar: calendar.Spec{TimeZone: "America/Sao_Paulo"}, Prefs: p}, err
		}, discard)
		return &tenant.Tenant{User: u, KV: kv, Accounts: accounts, Prefs: ps, Runner: runner}
	}, discard, 0)

	h.srv = httptest.NewServer(New(Deps{
		Tenants:  manager,
		Auth:     account.NewAuthenticator(client, host),
		Sessions: session.New(h.root),
		Token:    adminToken,
		AdminTenant: func(ctx context.Context) (*tenant.Tenant, error) {
			return manager.Register(ctx, tenant.User{ID: "admin"})
		},
		GoogleReady:  func(context.Context, store.KV) bool { return false },
		GoogleAuth:   h.google,
		CalendarName: "Prazo — UDESC",
		Log:          discard,
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) call(t *testing.T, token, method, path, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (h *harness) login(t *testing.T, user string) loginResponse {
	t.Helper()
	var lr loginResponse
	body := fmt.Sprintf(`{"baseUrl":%q,"username":%q,"password":"certa"}`, h.moodle.URL, user)
	if code := h.call(t, "", http.MethodPost, "/auth/login", body, &lr); code != http.StatusOK || lr.Token == "" {
		t.Fatalf("login %s: %d %+v", user, code, lr)
	}
	return lr
}

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
	var pa, pb prefs.Preferences
	h.call(t, ana.Token, http.MethodGet, "/api/v1/preferences", "", &pa)
	h.call(t, bia.Token, http.MethodGet, "/api/v1/preferences", "", &pb)
	if len(pa.HiddenCourses) != 1 || len(pb.HiddenCourses) != 0 {
		t.Fatalf("preferências vazaram: ana=%+v bia=%+v", pa, pb)
	}

	var la, lb DeadlineList
	h.call(t, ana.Token, http.MethodGet, "/api/v1/deadlines", "", &la)
	h.call(t, bia.Token, http.MethodGet, "/api/v1/deadlines", "", &lb)
	if len(la.Deadlines) != 1 || len(lb.Deadlines) != 1 || la.Deadlines[0].ID == lb.Deadlines[0].ID {
		t.Fatalf("prazos: ana=%+v bia=%+v", la.Deadlines, lb.Deadlines)
	}
	if !la.Deadlines[0].Hidden || lb.Deadlines[0].Hidden {
		t.Fatal("a preferência da ana afetou a bia")
	}

	var run Run
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
	var e errorBody
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
	var e errorBody
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
	var m Meta
	h.call(t, adminToken, http.MethodGet, "/api/v1/meta", "", &m)
	if len(m.Palette) != 11 || m.Palette[0].ID != "1" || len(m.Types) != len(domain.DeadlineTypes) {
		t.Fatalf("meta = %+v", m)
	}
}

type fakeGoogleAuth struct {
	startedBy string
}

func (f *fakeGoogleAuth) Start(userID, returnTo string) (string, error) {
	if strings.HasPrefix(returnTo, "http") {
		return "", errors.New("returnTo inválido")
	}
	f.startedBy = userID
	return "https://accounts.google.com/o/oauth2/auth?state=s1", nil
}

func (f *fakeGoogleAuth) Finish(_ context.Context, state, code string) (string, string, error) {
	if state != "s1" || code != "c1" {
		return "", "", errors.New("autorização expirada")
	}
	return f.startedBy, "exp://192.168.0.14:8081/--/google", nil
}

func (f *fakeGoogleAuth) Disconnect(context.Context, string) error { return nil }

func TestGoogleAppAuth(t *testing.T) {
	h := newHarness(t)
	ana := h.login(t, "ana")
	var auth googleAuthResponse
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
