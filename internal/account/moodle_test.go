package account

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/source"
	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
	"github.com/GabrielLima2803/prazo-sync/internal/source/webservice"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

const (
	password    = "senha-super-secreta"
	wsToken     = "ws-token-123"
	exportToken = "export-token-456"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}
func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m memKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

// moodle imita token.php, server.php e export_execute.php. pending são os ids
// devolvidos como "ainda exige ação".
type moodle struct {
	*httptest.Server
	mu         sync.Mutex
	withExport bool
	pending    []int64
}

func fakeMoodle(t *testing.T, withExport bool) *moodle {
	t.Helper()
	ics, err := os.ReadFile("../../testdata/moodle_sintetico.ics")
	if err != nil {
		t.Fatal(err)
	}
	m := &moodle{withExport: withExport}
	m.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/login/token.php":
			if r.Form.Get("password") != password {
				io.WriteString(w, `{"error":"Login inválido","errorcode":"invalidlogin"}`)
				return
			}
			io.WriteString(w, `{"token":"`+wsToken+`"}`)
		case "/webservice/rest/server.php":
			if r.Form.Get("wstoken") != wsToken {
				io.WriteString(w, `{"exception":"x","errorcode":"invalidtoken","message":"token"}`)
				return
			}
			switch r.Form.Get("wsfunction") {
			case "core_webservice_get_site_info":
				fns := `{"name":"core_webservice_get_site_info"}`
				if m.withExport {
					fns += `,{"name":"core_calendar_get_calendar_export_token"}`
				}
				io.WriteString(w, `{"sitename":"Moodle CCT","fullname":"Aluno Teste","userid":42,"functions":[`+fns+`]}`)
			case "core_calendar_get_calendar_export_token":
				io.WriteString(w, `{"token":"`+exportToken+`","warnings":[]}`)
			case "core_calendar_get_action_events_by_timesort":
				m.mu.Lock()
				var events []string
				for _, id := range m.pending {
					events = append(events, fmt.Sprintf(`{"id":%d,"name":"x"}`, id))
				}
				m.mu.Unlock()
				io.WriteString(w, `{"events":[`+strings.Join(events, ",")+`]}`)
			}
		case "/rejeitado":
			io.WriteString(w, "Invalid authentication")
		case "/calendar/export_execute.php":
			if r.URL.Query().Get("authtoken") != exportToken || r.URL.Query().Get("userid") != "42" {
				io.WriteString(w, "Invalid authentication")
				return
			}
			w.Write(ics)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *moodle) setPending(ids ...int64) {
	m.mu.Lock()
	m.pending = ids
	m.mu.Unlock()
}

func (m *moodle) http() *httpx.Client {
	return httpx.New(httpx.Options{Transport: m.Client().Transport, MaxRetries: 0})
}

func (m *moodle) host() string {
	u, _ := url.Parse(m.URL)
	return u.Host
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestLoginAndSave(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	auth := NewAuthenticator(srv.http(), srv.host())

	m, err := auth.Login(ctx, srv.URL, " aluno ", password)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != MethodLogin || m.UserID != 42 || m.FullName != "Aluno Teste" || m.Host() != strings.ToLower(srv.host()) {
		t.Fatalf("conta = %+v", m)
	}
	if !strings.Contains(m.ICalURL, "authtoken="+exportToken) || !strings.Contains(m.ICalURL, "userid=42") {
		t.Errorf("URL do iCal = %s", m.ICalURL)
	}

	kv := memKV{}
	svc := NewService(kv, srv.http(), discard, "")
	n, err := svc.Save(ctx, m)
	if err != nil || n == 0 {
		t.Fatalf("Save = %d, %v", n, err)
	}
	if bytes.Contains(kv[store.KeyMoodle], []byte(password)) {
		t.Fatal("a senha foi guardada")
	}
	deadlines, err := svc.Source().Fetch(ctx)
	if err != nil || len(deadlines) == 0 {
		t.Fatalf("Source: %d prazos, %v", len(deadlines), err)
	}
}

func TestLoginFailures(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	auth := NewAuthenticator(srv.http(), srv.host())
	if _, err := auth.Login(ctx, srv.URL, "aluno", "errada"); !errors.Is(err, webservice.ErrInvalidLogin) {
		t.Errorf("senha errada: %v", err)
	}
	if _, err := auth.Login(ctx, "http://inseguro", "a", "b"); !errors.Is(err, ErrInvalidBaseURL) {
		t.Errorf("http: %v", err)
	}
	if _, err := auth.Login(ctx, "https://outro-moodle.example", "a", "b"); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("host fora da lista: %v", err)
	}
	noExport := fakeMoodle(t, false)
	if _, err := NewAuthenticator(noExport.http(), noExport.host()).Login(ctx, noExport.URL, "aluno", password); !errors.Is(err, ErrExportUnavailable) {
		t.Errorf("sem a função de exportação: %v", err)
	}
}

func TestFromICal(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	auth := NewAuthenticator(srv.http(), srv.host())
	if _, err := auth.FromICal(srv.URL + "/calendar/export_execute.php?authtoken=x"); !errors.Is(err, ErrMissingUserID) {
		t.Fatalf("sem userid: %v", err)
	}

	kv := memKV{}
	svc := NewService(kv, srv.http(), discard, "")
	bad, err := auth.FromICal(srv.URL + "/calendar/export_execute.php?userid=42&authtoken=errado")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, bad); !errors.Is(err, ical.ErrAuthRejected) || len(kv) != 0 {
		t.Fatalf("URL recusada: %v (salvou: %v)", err, len(kv) != 0)
	}
	good, _ := auth.FromICal(srv.URL + "/calendar/export_execute.php?userid=42&authtoken=" + exportToken)
	if n, err := svc.Save(ctx, good); err != nil || n == 0 || good.UserID != 42 {
		t.Fatalf("Save = %d, %v", n, err)
	}
	if err := svc.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Moodle(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("depois de desconectar: %v", err)
	}
}

func TestEnvFallback(t *testing.T) {
	srv := fakeMoodle(t, true)
	env := srv.URL + "/calendar/export_execute.php?userid=42&authtoken=" + exportToken
	m, err := NewService(memKV{}, srv.http(), discard, env).Moodle(context.Background())
	if err != nil || m.Method != MethodEnv || m.BaseURL != srv.URL {
		t.Fatalf("fallback = %+v, %v", m, err)
	}
	if _, err := NewService(nil, srv.http(), discard, "").Moodle(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("sem nada: %v", err)
	}
	if host, id := IdentityFromICal(env); id != 42 || host != strings.ToLower(srv.host()) {
		t.Fatalf("identidade = %s %d", host, id)
	}
}

func TestRejectedTokenMarksReauth(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	kv := memKV{}
	svc := NewService(kv, srv.http(), discard, "")
	m, _ := NewAuthenticator(srv.http(), srv.host()).Login(ctx, srv.URL, "aluno", password)
	if _, err := svc.Save(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Simula a troca de senha: a URL salva passa a receber "Invalid authentication".
	m.ICalURL = srv.URL + "/rejeitado"
	svc.put(ctx, m)
	if _, err := svc.Source().Fetch(ctx); !errors.Is(err, ErrReauthRequired) || !errors.Is(err, source.ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if saved, _ := svc.Moodle(ctx); !saved.NeedsReauth {
		t.Fatal("a conta deveria ficar marcada")
	}

	fresh, _ := NewAuthenticator(srv.http(), srv.host()).Login(ctx, srv.URL, "aluno", password)
	if _, err := svc.Save(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if again, _ := svc.Moodle(ctx); again.NeedsReauth {
		t.Fatal("o novo login deveria limpar a marca")
	}
}

func TestSubmittedNeedsToBeSeenPendingFirst(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	kv := memKV{}
	svc := NewService(kv, srv.http(), discard, "")
	// O fixture sintético tem eventos em 2026; o relógio fica antes deles.
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	m, _ := NewAuthenticator(srv.http(), srv.host()).Login(ctx, srv.URL, "aluno", password)
	svc.Save(ctx, m)

	first, err := svc.Source().Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(first))
	for _, d := range first {
		if d.Submitted {
			t.Fatalf("%s marcado sem nunca ter aparecido como pendente", d.SourceID)
		}
		ids = append(ids, eventID(d.SourceID))
	}

	srv.setPending(ids...) // todos pendentes
	svc.Source().Fetch(ctx)
	srv.setPending(ids[1:]...) // o primeiro foi entregue
	got, _ := svc.Source().Fetch(ctx)
	for _, d := range got {
		if want := eventID(d.SourceID) == ids[0]; d.Submitted != want {
			t.Errorf("%s: Submitted = %v; quer %v", d.SourceID, d.Submitted, want)
		}
	}
}

func TestNoAllowedHostsRefusesEveryone(t *testing.T) {
	srv := fakeMoodle(t, true)
	if _, err := NewAuthenticator(srv.http()).Login(context.Background(), srv.URL, "aluno", password); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("sem hosts liberados: %v", err)
	}
}
