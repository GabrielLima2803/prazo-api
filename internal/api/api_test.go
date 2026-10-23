package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
)

const token = "token-de-teste-com-tamanho-suficiente"

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, store.ErrNotFound
}

func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }

func (m memKV) Delete(_ context.Context, k string) error { delete(m, k); return nil }

type fakeSource []domain.Deadline

func (f fakeSource) Fetch(context.Context) ([]domain.Deadline, error) { return f, nil }

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	at := func(h time.Duration) time.Time { return now.Add(h) }
	src := fakeSource{
		{SourceID: "1", CourseCode: "POO", CourseName: "2026/2_POO", Type: domain.TypeAssign, Phase: domain.PhaseDue, Title: "Aula 9", StartsAt: at(24 * time.Hour), EndsAt: at(24 * time.Hour)},
		{SourceID: "2", CourseCode: "CDI", CourseName: "2026/2_CDI", Type: domain.TypeExam, Phase: domain.PhaseDue, Title: "Prova 1", StartsAt: at(48 * time.Hour), EndsAt: at(50 * time.Hour)},
		{SourceID: "3", CourseCode: "POO", CourseName: "2026/2_POO", Type: domain.TypeAssign, Phase: domain.PhaseDue, Title: "Aula 1", StartsAt: at(-48 * time.Hour), EndsAt: at(-48 * time.Hour)},
	}
	svc := sync.NewService(src, domain.NormalizeOptions{}, sync.WithClock(func() time.Time { return now }))
	kv := memKV{}
	ps := prefs.NewStore(kv)
	sink := calendartest.New()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := sync.NewRunner(svc, func(ctx context.Context) (sync.Target, error) {
		p, err := ps.Load(ctx)
		return sync.Target{Sink: sink, State: kv, Calendar: calendar.Spec{TimeZone: "America/Sao_Paulo"}, Prefs: p}, err
	}, log)
	srv := httptest.NewServer(New(Deps{
		Runner: runner, Prefs: ps, Token: token, CalendarName: "Prazo — UDESC",
		Accounts:    account.NewService(kv, httpx.New(httpx.Options{}), log, ""),
		GoogleReady: func(context.Context) bool { return true }, Log: log,
	}))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func TestAuth(t *testing.T) {
	srv := newServer(t)
	for _, h := range []string{"", "Bearer errado", token} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/status", nil)
		req.Header.Set("Authorization", h)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q: %d", h, resp.StatusCode)
		}
	}
	resp, _ := http.Get(srv.URL + "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz = %d", resp.StatusCode)
	}
}

func TestDeadlinesAndCourses(t *testing.T) {
	srv := newServer(t)
	var list DeadlineList
	if code := call(t, srv, http.MethodGet, "/api/v1/deadlines", "", &list); code != http.StatusOK || len(list.Deadlines) != 3 {
		t.Fatalf("deadlines: %d %+v", code, list)
	}
	exam := list.Deadlines[2]
	if exam.ID != "2" || exam.Instant || exam.Color == "" || len(exam.Reminders) != 2 || exam.Label != "Prova" {
		t.Errorf("prova = %+v", exam)
	}

	var courses []Course
	call(t, srv, http.MethodGet, "/api/v1/courses", "", &courses)
	if len(courses) != 2 || courses[1].Code != "POO" || courses[1].Total != 2 || courses[1].Pending != 1 || courses[1].NextDue == nil {
		t.Fatalf("courses = %+v", courses)
	}
}

func TestPreferencesFlowIntoDeadlinesAndSync(t *testing.T) {
	srv := newServer(t)
	body := `{"hiddenCourses":["poo"],"courseColors":{"CDI":"11"},"reminders":{"exam":[60]}}`
	var saved prefs.Preferences
	if code := call(t, srv, http.MethodPut, "/api/v1/preferences", body, &saved); code != http.StatusOK || saved.HiddenCourses[0] != "POO" {
		t.Fatalf("put: %d %+v", code, saved)
	}

	var list DeadlineList
	call(t, srv, http.MethodGet, "/api/v1/deadlines", "", &list)
	for _, d := range list.Deadlines {
		if (d.CourseCode == "POO") != d.Hidden {
			t.Errorf("%s hidden = %v", d.ID, d.Hidden)
		}
		if d.CourseCode == "CDI" && (d.ColorID != "11" || len(d.Reminders) != 1 || d.Reminders[0] != 60) {
			t.Errorf("CDI sem personalização: %+v", d)
		}
	}

	var run Run
	if code := call(t, srv, http.MethodPost, "/api/v1/sync", "", &run); code != http.StatusOK || !run.OK || run.Created != 1 || run.Hidden != 2 {
		t.Fatalf("sync: %d %+v", code, run)
	}
	var st Status
	call(t, srv, http.MethodGet, "/api/v1/status", "", &st)
	if st.LastRun == nil || st.LastRun.Created != 1 || !st.Google.Authorized {
		t.Fatalf("status = %+v", st)
	}
}

func TestInvalidPreferences(t *testing.T) {
	srv := newServer(t)
	if code := call(t, srv, http.MethodPut, "/api/v1/preferences", `{"courseColors":{"POO":"99"}}`, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("cor inválida: %d", code)
	}
	if code := call(t, srv, http.MethodPut, "/api/v1/preferences", `{"campoDesconhecido":1}`, nil); code != http.StatusBadRequest {
		t.Errorf("campo desconhecido: %d", code)
	}
}

func TestMeta(t *testing.T) {
	srv := newServer(t)
	var m Meta
	call(t, srv, http.MethodGet, "/api/v1/meta", "", &m)
	if len(m.Palette) != 11 || m.Palette[0].ID != "1" || len(m.Types) != len(domain.DeadlineTypes) {
		t.Fatalf("meta = %+v", m)
	}
}

func TestMoodleEndpoints(t *testing.T) {
	srv := newServer(t)
	var st MoodleStatus
	if code := call(t, srv, http.MethodGet, "/api/v1/moodle", "", &st); code != http.StatusOK || st.Connected {
		t.Fatalf("sem conta: %d %+v", code, st)
	}
	var e errorBody
	if code := call(t, srv, http.MethodPost, "/api/v1/moodle/login", `{"baseUrl":"http://inseguro","username":"a","password":"b"}`, &e); code != http.StatusUnprocessableEntity || e.Code != "invalid_url" {
		t.Errorf("http: %d %+v", code, e)
	}
	if code := call(t, srv, http.MethodPost, "/api/v1/moodle/ical", `{"icalUrl":"ftp://x"}`, &e); code != http.StatusUnprocessableEntity || e.Code != "invalid_url" {
		t.Errorf("ftp: %d %+v", code, e)
	}
	if strings.Contains(e.Error, "ftp://x") {
		t.Error("a mensagem não pode ecoar a URL")
	}
}

func TestDeadlinesWithoutMoodle(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kv := memKV{}
	accounts := account.NewService(kv, httpx.New(httpx.Options{}), log, "")
	runner := sync.NewRunner(sync.NewService(accounts.Source(), domain.NormalizeOptions{}), nil, log)
	srv := httptest.NewServer(New(Deps{Runner: runner, Accounts: accounts, Prefs: prefs.NewStore(kv), Token: token, Log: log,
		GoogleReady: func(context.Context) bool { return false }}))
	defer srv.Close()

	var e errorBody
	if code := call(t, srv, http.MethodGet, "/api/v1/deadlines", "", &e); code != http.StatusConflict || e.Code != "moodle_not_connected" {
		t.Fatalf("deadlines sem Moodle: %d %+v", code, e)
	}
}

type fakeGoogleAuth struct {
	finished bool
}

func (f *fakeGoogleAuth) Start(returnTo string) (string, error) {
	if strings.HasPrefix(returnTo, "http") {
		return "", errors.New("returnTo inválido")
	}
	return "https://accounts.google.com/o/oauth2/auth?state=s1", nil
}

func (f *fakeGoogleAuth) Finish(_ context.Context, state, code string) (string, error) {
	if state != "s1" || code != "c1" {
		return "", errors.New("autorização expirada")
	}
	f.finished = true
	return "exp://192.168.0.14:8081/--/google", nil
}

func (f *fakeGoogleAuth) Disconnect(context.Context) error { return nil }

func TestGoogleAppAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kv := memKV{}
	accounts := account.NewService(kv, httpx.New(httpx.Options{}), log, "")
	runner := sync.NewRunner(sync.NewService(accounts.Source(), domain.NormalizeOptions{}), func(context.Context) (sync.Target, error) {
		return sync.Target{}, errors.New("sem destino no teste")
	}, log)
	ga := &fakeGoogleAuth{}
	srv := httptest.NewServer(New(Deps{Runner: runner, Accounts: accounts, Prefs: prefs.NewStore(kv), Token: token, Log: log,
		GoogleAuth: ga, GoogleReady: func(context.Context) bool { return ga.finished }}))
	defer srv.Close()

	var st Status
	call(t, srv, http.MethodGet, "/api/v1/status", "", &st)
	if !st.Google.AppAuth || st.Google.Authorized {
		t.Fatalf("antes: %+v", st.Google)
	}
	var auth googleAuthResponse
	if code := call(t, srv, http.MethodPost, "/api/v1/google/auth", `{"returnTo":"exp://192.168.0.14:8081/--/google"}`, &auth); code != http.StatusOK || auth.AuthURL == "" {
		t.Fatalf("start: %d %+v", code, auth)
	}

	resp, err := http.Get(srv.URL + "/oauth/google/callback?state=s1&code=c1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `href="exp://192.168.0.14:8081/--/google?google=ok"`) {
		t.Fatalf("callback: %d\n%s", resp.StatusCode, body)
	}

	resp, _ = http.Get(srv.URL + "/oauth/google/callback?state=forjado&code=c1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("state forjado: %d", resp.StatusCode)
	}
}

func TestGoogleAppAuthUnavailable(t *testing.T) {
	srv := newServer(t)
	var e errorBody
	if code := call(t, srv, http.MethodPost, "/api/v1/google/auth", `{}`, &e); code != http.StatusNotImplemented || e.Code != "google_app_auth_unavailable" {
		t.Fatalf("sem configuração: %d %+v", code, e)
	}
}

func TestLoginIssuesSession(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kv := memKV{}
	moodle := fakeMoodleTLS(t)
	h := httpx.New(httpx.Options{Transport: moodle.Client().Transport})
	accounts := account.NewService(kv, h, log, "")
	sessions := session.New(kv)
	runner := sync.NewRunner(sync.NewService(accounts.Source(), domain.NormalizeOptions{}), func(context.Context) (sync.Target, error) {
		return sync.Target{}, errors.New("sem destino no teste")
	}, log)
	srv := httptest.NewServer(New(Deps{Runner: runner, Accounts: accounts, Sessions: sessions, Prefs: prefs.NewStore(kv), Token: token, Log: log,
		GoogleReady: func(context.Context) bool { return false }}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"baseUrl":"`+moodle.URL+`","username":"aluno","password":"certa"}`))
	if err != nil {
		t.Fatal(err)
	}
	var lr loginResponse
	json.NewDecoder(resp.Body).Decode(&lr)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || lr.Token == "" || !lr.Moodle.Connected {
		t.Fatalf("login: %d %+v", resp.StatusCode, lr)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/moodle", nil)
	req.Header.Set("Authorization", "Bearer "+lr.Token)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sessão não aceita: %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+lr.Token)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if sessions.Valid(context.Background(), lr.Token) {
		t.Fatal("logout não revogou a sessão")
	}
}

// fakeMoodleTLS é o mínimo para o login: token.php, site info, token de
// exportação e um calendário com um evento.
func fakeMoodleTLS(t *testing.T) *httptest.Server {
	t.Helper()
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1@moodle\r\nSUMMARY:Lista 1 é devido\r\nDTSTART:20261030T025900Z\r\nDTEND:20261030T025900Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch {
		case r.URL.Path == "/login/token.php":
			io.WriteString(w, `{"token":"ws"}`)
		case r.Form.Get("wsfunction") == "core_webservice_get_site_info":
			io.WriteString(w, `{"sitename":"M","fullname":"Aluno","userid":42,"functions":[{"name":"core_calendar_get_calendar_export_token"}]}`)
		case r.Form.Get("wsfunction") == "core_calendar_get_calendar_export_token":
			io.WriteString(w, `{"token":"exp"}`)
		case r.URL.Path == "/calendar/export_execute.php":
			io.WriteString(w, ics)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
