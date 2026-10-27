package tests

import (
	"github.com/GabrielLima2803/prazo-sync/internal/api"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar/calendartest"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
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
	return nil, storage.ErrNotFound
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
	// forgotten registra os KVs cujo token do Google foi revogado.
	forgotten int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{moodle: fakeMoodle(t), root: newMemKV(), sinks: map[string]*calendartest.Fake{}, google: &fakeGoogleAuth{}}
	client := httpx.New(httpx.Options{Transport: h.moodle.Client().Transport})
	host := strings.TrimPrefix(h.moodle.URL, "https://")

	manager := tenant.NewManager(h.root, func(u tenant.User, kv storage.KV) *tenant.Tenant {
		accounts := moodle.NewAccountService(kv, client, discard, "")
		ps := sync.NewPreferenceStore(kv)
		sink := calendartest.New()
		h.sinks[u.ID] = sink
		runner := sync.NewRunner(sync.NewService(accounts.Source(), domain.NormalizeOptions{}), func(ctx context.Context) (sync.Target, error) {
			p, err := ps.Load(ctx)
			return sync.Target{Sink: sink, State: kv, Calendar: calendar.Spec{TimeZone: "America/Sao_Paulo"}, Prefs: p}, err
		}, discard)
		return &tenant.Tenant{User: u, KV: kv, Accounts: accounts, Prefs: ps, Runner: runner}
	}, discard, 0)

	h.srv = httptest.NewServer(api.New(api.Deps{
		Tenants:  manager,
		Auth:     moodle.NewAuthenticator(client, host),
		Sessions: tenant.NewSessions(h.root),
		Token:    adminToken,
		AdminTenant: func(ctx context.Context) (*tenant.Tenant, error) {
			return manager.Register(ctx, tenant.User{ID: "admin"})
		},
		GoogleReady: func(context.Context, storage.KV) bool { return false },
		GoogleAuth:  h.google,
		ForgetGoogle: func(ctx context.Context, kv storage.KV) error {
			h.forgotten++
			return kv.Delete(ctx, storage.KeyGoogleToken)
		},
		CORSOrigins:  []string{"http://localhost:8081"},
		Privacy:      api.Privacy{Operator: "Fulano", Contact: "privacidade@example.com"},
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

func (h *harness) login(t *testing.T, user string) api.LoginResponse {
	t.Helper()
	var lr api.LoginResponse
	body := fmt.Sprintf(`{"baseUrl":%q,"username":%q,"password":"certa"}`, h.moodle.URL, user)
	if code := h.call(t, "", http.MethodPost, "/auth/login", body, &lr); code != http.StatusOK || lr.Token == "" {
		t.Fatalf("login %s: %d %+v", user, code, lr)
	}
	return lr
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
