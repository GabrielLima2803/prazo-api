package doctor

import (
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"

	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"
)

func loadFeed(t *testing.T, path string) moodle.Feed {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	feed, err := moodle.ParseICal(f)
	if err != nil {
		t.Fatal(err)
	}
	return feed
}

func TestAnalyzeSyntheticFixture(t *testing.T) {
	feed := loadFeed(t, "../../tests/testdata/moodle_sintetico.ics")
	feed.Events = append(feed.Events, feed.Events[0])

	r := AnalyzeICal(feed, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), 30*24*time.Hour)

	if r.Total != 10 || r.Invalid != 1 {
		t.Errorf("Total=%d Invalid=%d", r.Total, r.Invalid)
	}
	if len(r.DuplicateUIDs) != 1 {
		t.Errorf("DuplicateUIDs = %v", r.DuplicateUIDs)
	}
	if r.Rules[noRule] != 1 || r.Unmatched[0] != "Semana Acadêmica" {
		t.Errorf("Rules=%v Unmatched=%v", r.Rules, r.Unmatched)
	}
	for i := 1; i < len(r.Upcoming); i++ {
		if r.Upcoming[i].At.Before(r.Upcoming[i-1].At) {
			t.Fatal("Upcoming não está ordenado")
		}
	}

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Print(&buf, loc, 0)
	if !strings.Contains(buf.String(), "Lista 3") {
		t.Errorf("relatório sem a tabela de próximos:\n%s", buf.String())
	}
}

func TestAnalyzeRealCCTFixture(t *testing.T) {
	feed := loadFeed(t, "../../tests/testdata/moodle_cct_anon.ics")
	r := AnalyzeICal(feed, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), 30*24*time.Hour)

	// Só o evento pessoal (sem CATEGORIES) não usa template do Moodle.
	if r.Rules[noRule] != 1 || r.NoCategory != 1 {
		t.Errorf("Rules=%v NoCategory=%d", r.Rules, r.NoCategory)
	}
	if r.UIDShapes["#@moodle.joinville.udesc.br"] != 12 {
		t.Errorf("UIDShapes = %v", r.UIDShapes)
	}
}

func TestInspectWebService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.URL.Path == "/lib/ajax/service-nologin.php":
			_, _ = w.Write([]byte(`[{"error":false,"data":{"sitename":"Moodle CCT","typeoflogin":1}}]`))
		case r.PostForm.Get("wsfunction") == "core_webservice_get_site_info":
			_, _ = w.Write([]byte(`{"userid":1,"release":"4.5","functions":[{"name":"core_enrol_get_users_courses"}]}`))
		case r.PostForm.Get("wsfunction") == "core_enrol_get_users_courses":
			_, _ = w.Write([]byte(`[{"id":1,"shortname":"2026/2_CCT_CCI251-03A_POO0002","fullname":"POO"}]`))
		default:
			t.Errorf("chamada inesperada: %s %v", r.URL.Path, r.PostForm.Get("wsfunction"))
		}
	}))
	defer srv.Close()

	ws := moodle.NewWebServiceClient(srv.URL, httpx.New(httpx.Options{}))
	r := InspectWebService(context.Background(), ws, WebServiceOptions{Token: "TKN", Now: time.Now()})

	if r.PublicConfig.Err != nil || r.Site.Err != nil {
		t.Fatalf("erros: %v / %v", r.PublicConfig.Err, r.Site.Err)
	}
	if r.TokenProbe.Ran {
		t.Error("probe de login não foi pedido")
	}
	if r.Pending.Ran {
		t.Error("função de eventos não liberada não deveria ser chamada")
	}
	if len(r.Courses.Value) != 1 || r.Courses.Value[0].Code != "POO" {
		t.Errorf("Courses = %+v", r.Courses)
	}

	var buf bytes.Buffer
	r.Print(&buf)
	if !strings.Contains(buf.String(), "[POO]") {
		t.Errorf("saída sem a disciplina:\n%s", buf.String())
	}
}
