package moodle

import (
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"

	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *WebServiceClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewWebServiceClient(srv.URL+"/", httpx.New(httpx.Options{}))
}

func TestProbeTokenEndpoint(t *testing.T) {
	cases := map[string]bool{"invalidlogin": true, "servicenotavailable": false, "enablewsdescription": false}
	for code, want := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				t.Errorf("credenciais não podem ir na URL: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"error":"x","errorcode":"` + code + `"}`))
		})
		p, err := c.ProbeTokenEndpoint(context.Background())
		if err != nil || p.Available != want {
			t.Errorf("%s: Available=%v err=%v; quer %v", code, p.Available, err, want)
		}
	}
}

func TestLoginErrors(t *testing.T) {
	cases := map[string]error{"servicenotavailable": ErrServiceUnavailable, "invalidlogin": ErrInvalidLogin}
	for code, want := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"error":"x","errorcode":"` + code + `"}`))
		})
		if _, err := c.Login(context.Background(), "aluno", "senha"); !errors.Is(err, want) {
			t.Errorf("%s: esperava %v, veio %v", code, want, err)
		}
	}
}

func TestLoginOK(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("service") != MobileService || r.PostForm.Get("password") != "senha" {
			t.Errorf("form inesperado: %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"token":"TKN"}`))
	})
	if tok, err := c.Login(context.Background(), "aluno", "senha"); err != nil || tok != "TKN" {
		t.Fatalf("token=%q err=%v", tok, err)
	}
}

func TestCallMoodleException(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("wstoken") != "TKN" || strings.Contains(r.URL.RawQuery, "TKN") {
			t.Error("token deve ir no corpo do POST")
		}
		_, _ = w.Write([]byte(`{"exception":"webservice_access_exception","errorcode":"accessexception","message":"Acesso negado"}`))
	})
	_, err := c.SiteInfo(context.Background(), "TKN")
	var me *WebServiceError
	if !errors.As(err, &me) || me.Code != "accessexception" {
		t.Fatalf("esperava MoodleError, veio %v", err)
	}
}

func TestPublicConfigAndPendingEvents(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lib/ajax/service-nologin.php":
			_, _ = w.Write([]byte(`[{"error":false,"data":{"sitename":"Moodle CCT","typeoflogin":1,"enablewebservices":1}}]`))
		case "/webservice/rest/server.php":
			_ = r.ParseForm()
			if r.PostForm.Get("wsfunction") != "core_calendar_get_action_events_by_timesort" {
				t.Errorf("função inesperada %q", r.PostForm.Get("wsfunction"))
			}
			_, _ = w.Write([]byte(`{"events":[{"id":99677,"name":"Aula Prática 9","modulename":"assign","course":{"shortname":"POO"}}]}`))
		}
	})

	pc, err := c.PublicConfig(context.Background())
	if err != nil || pc.SiteName != "Moodle CCT" || pc.TypeOfLogin != LoginApp {
		t.Fatalf("PublicConfig = %+v, %v", pc, err)
	}
	evs, err := c.PendingEvents(context.Background(), "TKN", time.Now(), 50)
	if err != nil || len(evs) != 1 || evs[0].ID != 99677 || evs[0].ModuleName != "assign" {
		t.Fatalf("PendingEvents = %+v, %v", evs, err)
	}
}

func TestLoginErrorAuthBackendDown(t *testing.T) {
	if err := loginError("auth_ldap_noconnect_all", "LDAP - O módulo não conseguiu se conectar"); !errors.Is(err, ErrAuthBackendDown) {
		t.Fatalf("err = %v", err)
	}
}
