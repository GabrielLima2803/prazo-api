package doctor

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // não depender do tzdata do sistema (CI/alpine)

	"prazo/internal/source/ical"
)

func TestCounterTopIsDeterministic(t *testing.T) {
	c := Counter{}
	for _, k := range []string{"b", "a", "c", "a", "b", "a"} {
		c.Add(k)
	}
	got := c.Top(2)
	if len(got) != 2 || got[0] != (Entry{"a", 3}) || got[1] != (Entry{"b", 2}) {
		t.Fatalf("Top(2) = %v", got)
	}
}

func TestAnalyzeFixture(t *testing.T) {
	f, err := os.Open("../../testdata/moodle_sintetico.ics")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	events, skipped, err := ical.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	// Evento duplicado proposital para exercitar a detecção.
	events = append(events, events[0])

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	r := Analyze(events, skipped, now, 30*24*time.Hour)

	if r.Total != 10 || r.Skipped != 1 {
		t.Errorf("Total=%d Skipped=%d", r.Total, r.Skipped)
	}
	if len(r.DuplicateUIDs) != 1 {
		t.Errorf("DuplicateUIDs = %v", r.DuplicateUIDs)
	}
	if r.UIDShapes["#@www.moodle.udesc.br"] != 10 {
		t.Errorf("UIDShapes = %v", r.UIDShapes)
	}
	if r.Rules["(nenhuma)"] != 1 || r.Unmatched[0] != "Semana Acadêmica" {
		t.Errorf("Rules=%v Unmatched=%v", r.Rules, r.Unmatched)
	}
	for i := 1; i < len(r.Upcoming); i++ {
		if r.Upcoming[i].Start.Before(r.Upcoming[i-1].Start) {
			t.Fatal("Upcoming não está ordenado")
		}
	}

	var buf bytes.Buffer
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	r.Print(&buf, loc, 0)
	if !strings.Contains(buf.String(), "Lista 3") {
		t.Errorf("relatório sem a tabela de próximos:\n%s", buf.String())
	}
}

func TestAnalyzeRealCCTFixture(t *testing.T) {
	f, err := os.Open("../../testdata/moodle_cct_anon.ics")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	events, skipped, err := ical.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	r := Analyze(events, skipped, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), 30*24*time.Hour)

	// Só o evento pessoal (sem CATEGORIES) não tem template do Moodle.
	if r.Rules["(nenhuma)"] != 1 || r.NoCategory != 1 {
		t.Errorf("Rules=%v NoCategory=%d", r.Rules, r.NoCategory)
	}
	if r.UIDShapes["#@moodle.joinville.udesc.br"] != 12 {
		t.Errorf("UIDShapes = %v", r.UIDShapes)
	}
}
