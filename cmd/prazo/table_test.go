package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

var now = time.Date(2026, 10, 20, 22, 0, 0, 0, time.UTC)

func TestPrintDeadlines(t *testing.T) {
	ds := []domain.Deadline{{
		CourseCode: "POO", Title: "Aula Prática 10", Type: domain.TypeAssign, Phase: domain.PhaseDue,
		StartsAt: now.Add(52 * time.Hour), URL: "https://m/x",
	}}
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	printDeadlines(&buf, ds, now, loc, true)
	for _, want := range []string{"qui 22/10 23:00", "2d 4h", "[POO] Entrega: Aula Prática 10", "https://m/x"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("tabela sem %q:\n%s", want, buf.String())
		}
	}
}

func TestTimeLeft(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Minute:               "vencido",
		30 * time.Minute:           "30min",
		5 * time.Hour:              "5h",
		48 * time.Hour:             "2d",
		50*time.Hour + time.Minute: "2d 2h",
	}
	for d, want := range cases {
		if got := timeLeft(now, now.Add(d)); got != want {
			t.Errorf("timeLeft(%v) = %q; quer %q", d, got, want)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	if code := run([]string{"voar"}); code != 2 {
		t.Errorf("exit code = %d; quer 2", code)
	}
}
