package ical

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
)

var t0 = time.Date(2026, 11, 17, 5, 5, 2, 0, time.UTC)

func TestToDeadline(t *testing.T) {
	e := Event{
		UID:         "99677@moodle.joinville.udesc.br",
		Summary:     "Aula Prática 9 - Tratamento de Exceções em Java está marcado(a) para esta data",
		Description: "Aula Prática 9 - Tratamento de Exceções em Java [1]\n\n\n\nLinks:\n------\n[1] https://moodle.joinville.udesc.br/mod/assign/view.php?id=364845\n",
		Categories:  []string{"2026/2_CCT_CCI251-03A_POO0002"},
		Start:       t0,
		End:         t0,
	}
	d := ToDeadline(e, "https://moodle.joinville.udesc.br")
	if d.CourseCode != "POO" || d.Type != domain.TypeAssign || d.Phase != domain.PhaseDue {
		t.Errorf("classificação: %+v", d)
	}
	if d.Title != "Aula Prática 9 - Tratamento de Exceções em Java" {
		t.Errorf("Title = %q", d.Title)
	}
	if d.URL != "https://moodle.joinville.udesc.br/mod/assign/view.php?id=364845" {
		t.Errorf("URL = %q", d.URL)
	}
	if d.Description != "" {
		t.Errorf("descrição que é só o nome autolinkado deveria sumir: %q", d.Description)
	}
}

func TestToDeadlineFallbacks(t *testing.T) {
	e := Event{
		UID:         "1@x",
		Summary:     "Reunião de turma",
		Description: "Ver slides [1] antes.\n\nLinks:\n------\n[1] https://x/mod/resource/view.php?id=9\n",
		Categories:  []string{"2026/2_CCT_CCI251-03A_AMS0003"},
		Start:       t0,
		End:         t0,
	}
	d := ToDeadline(e, "https://x")
	if !strings.HasPrefix(d.URL, "https://x/calendar/view.php?view=day&time=") {
		t.Errorf("link de slide não é a atividade; esperava link do calendário, veio %q", d.URL)
	}
	if d.Description != "Ver slides antes." {
		t.Errorf("Description = %q", d.Description)
	}
	if d.Type != domain.TypeCourseEvent {
		t.Errorf("evento de disciplina sem template deveria ser course_event, veio %s", d.Type)
	}
}

func TestClientRejectsHTMLAndBadURL(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>login</html>"))
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL, httpx.New(httpx.Options{Transport: srv.Client().Transport}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Fetch(context.Background()); !errors.Is(err, ErrNotICalendar) {
		t.Errorf("esperava ErrNotICalendar, veio %v", err)
	}
	if _, err := NewClient("http://inseguro/x", nil); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("esperava ErrInvalidURL, veio %v", err)
	}
}

func TestSourceFetchRealFixture(t *testing.T) {
	body, err := os.ReadFile("../../../testdata/moodle_cct_anon.ics")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL+"/calendar/export_execute.php?authtoken=x", httpx.New(httpx.Options{Transport: srv.Client().Transport}))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := NewSource(client, slog.New(slog.NewTextHandler(io.Discard, nil))).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 12 {
		t.Fatalf("esperava 12, veio %d", len(ds))
	}
	// Na fixture há Início + Término + "deve estar concluído" de tarefas do CDI;
	// depois de normalizar, cada atividade aparece uma vez só.
	seen := map[string]int{}
	for _, d := range domain.Normalize(ds, domain.NormalizeOptions{}) {
		seen[d.CourseName+"|"+d.Title]++
		if d.Phase == domain.PhaseOpen && strings.Contains(d.Title, "TAREFA 9") {
			t.Error("Início da TAREFA 9 deveria ter sido agrupado no Término")
		}
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("atividade duplicada após normalizar: %s (%d)", k, n)
		}
	}
}
