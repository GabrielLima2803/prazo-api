package sync

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"prazo/internal/domain"
)

// fakeSource implementa source.Source para testes.
type fakeSource struct {
	ds  []domain.Deadline
	err error
}

func (f fakeSource) Fetch(context.Context) ([]domain.Deadline, error) { return f.ds, f.err }

var now = time.Date(2026, 10, 20, 22, 0, 0, 0, time.UTC)

func TestPlanNormalizes(t *testing.T) {
	src := fakeSource{ds: []domain.Deadline{
		{SourceID: "2", CourseCode: "CDI", CourseName: "CDI", Title: "T9", Type: domain.TypeAssign, Phase: domain.PhaseClose, StartsAt: now.Add(48 * time.Hour)},
		{SourceID: "1", CourseCode: "CDI", CourseName: "CDI", Title: "T9", Type: domain.TypeAssign, Phase: domain.PhaseOpen, StartsAt: now},
	}}
	s := &Service{Source: src}
	ds, fetched, err := s.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 2 || len(ds) != 1 || ds[0].SourceID != "2" || ds[0].ContentHash == "" {
		t.Fatalf("fetched=%d ds=%+v", fetched, ds)
	}
}

func TestPlanPropagatesError(t *testing.T) {
	s := &Service{Source: fakeSource{err: errors.New("moodle fora do ar")}}
	if _, _, err := s.Plan(context.Background()); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestWindowAndTable(t *testing.T) {
	ds := []domain.Deadline{
		{SourceID: "a", CourseCode: "POO", Title: "Aula Prática 9", Type: domain.TypeAssign, Phase: domain.PhaseDue, StartsAt: now.Add(-time.Hour)},
		{SourceID: "b", CourseCode: "POO", Title: "Aula Prática 10", Type: domain.TypeAssign, Phase: domain.PhaseDue, StartsAt: now.Add(52 * time.Hour), URL: "https://m/x"},
		{SourceID: "c", CourseCode: "EDA", Title: "Tarefa 2", Type: domain.TypeAssign, Phase: domain.PhaseDue, StartsAt: now.Add(40 * 24 * time.Hour)},
	}
	w := Window(ds, now, now.Add(30*24*time.Hour))
	if len(w) != 1 || w[0].SourceID != "b" {
		t.Fatalf("Window = %+v", w)
	}

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	var buf bytes.Buffer
	PrintTable(&buf, w, now, loc, true)
	out := buf.String()
	for _, want := range []string{"qui 22/10 23:00", "2d 4h", "[POO] Entrega: Aula Prática 10", "https://m/x"} {
		if !strings.Contains(out, want) {
			t.Errorf("tabela sem %q:\n%s", want, out)
		}
	}
}

func TestUntil(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Minute:               "vencido",
		30 * time.Minute:           "30min",
		5 * time.Hour:              "5h",
		48 * time.Hour:             "2d",
		50*time.Hour + time.Minute: "2d 2h",
	}
	for d, want := range cases {
		if got := Until(now, now.Add(d)); got != want {
			t.Errorf("Until(%v) = %q; quer %q", d, got, want)
		}
	}
}
