package calendar

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

var due = time.Date(2026, 10, 22, 2, 59, 0, 0, time.UTC)

func deadline() domain.Deadline {
	return domain.Deadline{
		SourceID: "4321@moodle.joinville.udesc.br", CourseCode: "CDI", CourseName: "2026/2_CCT_CCI251-03U_CDI2002",
		Type: domain.TypeAssign, Phase: domain.PhaseClose, Title: "TAREFA 9", URL: "https://moodle.example/mod/quiz/view.php?id=1",
		StartsAt: due, EndsAt: due, ContentHash: "h",
	}
}

func TestEventIDIsValidForGoogle(t *testing.T) {
	valid := regexp.MustCompile(`^[a-v0-9]+$`)
	for _, id := range []string{"1@x", "4321@moodle.joinville.udesc.br", strings.Repeat("z", 5000)} {
		got := EventID(id)
		if !valid.MatchString(got) || len(got) < 5 || len(got) > 1024 {
			t.Errorf("EventID(%q) = %q: fora do alfabeto do Google", id, got)
		}
		if got != EventID(id) {
			t.Error("EventID não é determinístico")
		}
	}
	if EventID("1@x") == EventID("2@x") {
		t.Error("ids diferentes colidiram")
	}
}

func TestColorIDStable(t *testing.T) {
	if ColorID("") != "" {
		t.Error("sem disciplina deveria usar a cor da agenda")
	}
	c := ColorID("POO")
	if c != ColorID("poo") || c < "1" || len(c) > 2 {
		t.Errorf("ColorID = %q", c)
	}
}

func TestInstantEndsAtDeadline(t *testing.T) {
	e := FromDeadline(deadline(), "America/Sao_Paulo")
	if !e.End.Equal(due) || e.End.Sub(e.Start) != InstantDuration {
		t.Fatalf("janela = %v → %v", e.Start, e.End)
	}
	if e.Summary != "[CDI] Entrega: TAREFA 9" || e.Busy || len(e.Reminders) != 3 {
		t.Errorf("evento = %+v", e)
	}
	if e.Properties[PropSourceID] != "4321@moodle.joinville.udesc.br" || e.Properties[PropContentHash] != "h" {
		t.Errorf("properties = %v", e.Properties)
	}
	if !strings.Contains(e.Description, "Abrir no Moodle: https://moodle.example") {
		t.Errorf("descrição sem link: %q", e.Description)
	}
}

func TestIntervalKeepsRealTime(t *testing.T) {
	d := deadline()
	d.Type = domain.TypeExam
	d.StartsAt = due.Add(-2 * time.Hour)
	e := FromDeadline(d, "America/Sao_Paulo")
	if !e.Start.Equal(d.StartsAt) || !e.End.Equal(due) || !e.Busy {
		t.Fatalf("evento = %+v", e)
	}
	if got := e.Reminders; len(got) != 2 || got[0] != 7*day {
		t.Errorf("lembretes de prova = %v", got)
	}
}

func TestSubmittedHasNoReminders(t *testing.T) {
	d := deadline()
	d.Submitted = true
	if e := FromDeadline(d, ""); e.Reminders != nil || !strings.HasPrefix(e.Summary, "✅") {
		t.Fatalf("evento = %+v", e)
	}
}
