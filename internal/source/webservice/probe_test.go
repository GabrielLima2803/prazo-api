package webservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"prazo/internal/httpx"
)

func newServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: httpx.New(httpx.Options{})}
}

func TestProbeTokenEndpoint(t *testing.T) {
	cases := map[string]bool{"invalidlogin": true, "servicenotavailable": false, "enablewsdescription": false}
	for code, want := range cases {
		c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestLoginServiceUnavailable(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"Web services desabilitado","errorcode":"servicenotavailable"}`))
	})
	_, err := c.Login(context.Background(), "aluno", "senha")
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("esperava ErrServiceUnavailable, veio %v", err)
	}
}

func TestCallMoodleException(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("wstoken") != "TKN" || strings.Contains(r.URL.RawQuery, "TKN") {
			t.Error("token deve ir no corpo do POST")
		}
		_, _ = w.Write([]byte(`{"exception":"webservice_access_exception","errorcode":"accessexception","message":"Acesso negado"}`))
	})
	var out SiteInfo
	err := c.Call(context.Background(), "TKN", "core_webservice_get_site_info", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "accessexception") {
		t.Fatalf("esperava erro do moodle, veio %v", err)
	}
}
