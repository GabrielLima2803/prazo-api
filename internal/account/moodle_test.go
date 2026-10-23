package account

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
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

// fakeMoodle imita token.php, server.php e export_execute.php.
func fakeMoodle(t *testing.T, withExport bool) *httptest.Server {
	t.Helper()
	ics, err := os.ReadFile("../../testdata/moodle_sintetico.ics")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				if withExport {
					fns += `,{"name":"core_calendar_get_calendar_export_token"}`
				}
				io.WriteString(w, `{"sitename":"Moodle CCT","fullname":"Aluno Teste","userid":42,"functions":[`+fns+`]}`)
			case "core_calendar_get_calendar_export_token":
				io.WriteString(w, `{"token":"`+exportToken+`","warnings":[]}`)
			}
		case "/calendar/export_execute.php":
			if r.URL.Query().Get("authtoken") != exportToken || r.URL.Query().Get("userid") != "42" {
				io.WriteString(w, "<html>login</html>")
				return
			}
			w.Write(ics)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(srv *httptest.Server, kv store.KV, env string) *Service {
	h := httpx.New(httpx.Options{Transport: srv.Client().Transport, MaxRetries: 0})
	return NewService(kv, h, slog.New(slog.NewTextHandler(io.Discard, nil)), env)
}

func TestConnectWithLogin(t *testing.T) {
	srv := fakeMoodle(t, true)
	kv := memKV{}
	s := newService(srv, kv, "")

	m, events, err := s.ConnectWithLogin(context.Background(), srv.URL, " aluno ", password)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != MethodLogin || m.UserID != 42 || m.FullName != "Aluno Teste" || events == 0 {
		t.Fatalf("conta = %+v, eventos = %d", m, events)
	}
	if !strings.Contains(m.ICalURL, "authtoken="+exportToken) || !strings.Contains(m.ICalURL, "userid=42") {
		t.Errorf("URL do iCal = %s", m.ICalURL)
	}
	if bytes.Contains(kv[store.KeyMoodle], []byte(password)) {
		t.Fatal("a senha foi guardada")
	}

	deadlines, err := s.Source().Fetch(context.Background())
	if err != nil || len(deadlines) == 0 {
		t.Fatalf("Source com a conta salva: %d prazos, %v", len(deadlines), err)
	}
}

func TestConnectWithLoginFailures(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	kv := memKV{}
	if _, _, err := newService(srv, kv, "").ConnectWithLogin(ctx, srv.URL, "aluno", "errada"); !errors.Is(err, webservice.ErrInvalidLogin) {
		t.Errorf("senha errada: %v", err)
	}
	if _, _, err := newService(srv, kv, "").ConnectWithLogin(ctx, "http://inseguro", "a", "b"); !errors.Is(err, ErrInvalidBaseURL) {
		t.Errorf("http: %v", err)
	}
	noExport := fakeMoodle(t, false)
	if _, _, err := newService(noExport, kv, "").ConnectWithLogin(ctx, noExport.URL, "aluno", password); !errors.Is(err, ErrExportUnavailable) {
		t.Errorf("sem a função de exportação: %v", err)
	}
	if len(kv) != 0 {
		t.Fatal("falhas não podem salvar conta")
	}
}

func TestConnectWithICal(t *testing.T) {
	ctx := context.Background()
	srv := fakeMoodle(t, true)
	kv := memKV{}
	s := newService(srv, kv, "")

	bad := srv.URL + "/calendar/export_execute.php?userid=42&authtoken=errado"
	if _, _, err := s.ConnectWithICal(ctx, bad); !errors.Is(err, ical.ErrNotICalendar) || len(kv) != 0 {
		t.Fatalf("URL inválida: %v (salvou: %v)", err, len(kv) != 0)
	}
	good := srv.URL + "/calendar/export_execute.php?userid=42&authtoken=" + exportToken
	m, n, err := s.ConnectWithICal(ctx, good)
	if err != nil || m.Method != MethodICal || n == 0 {
		t.Fatalf("conta = %+v, %d, %v", m, n, err)
	}

	if err := s.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Moodle(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("depois de desconectar: %v", err)
	}
}

func TestEnvFallback(t *testing.T) {
	srv := fakeMoodle(t, true)
	env := srv.URL + "/calendar/export_execute.php?userid=42&authtoken=" + exportToken
	m, err := newService(srv, memKV{}, env).Moodle(context.Background())
	if err != nil || m.Method != MethodEnv || m.BaseURL != srv.URL {
		t.Fatalf("fallback = %+v, %v", m, err)
	}
	if _, err := newService(srv, nil, "").Moodle(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("sem nada: %v", err)
	}
}
