package ical

import (
	"os"
	"testing"
	"time"
)

func TestParseFixture(t *testing.T) {
	f, err := os.Open("../../../testdata/moodle_sintetico.ics")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	events, skipped, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 9 {
		t.Fatalf("esperava 9 eventos, veio %d", len(events))
	}
	if len(skipped) != 1 {
		t.Fatalf("esperava 1 evento ignorado (sem DTSTART), veio %d", len(skipped))
	}

	byUID := make(map[string]Event, len(events))
	for _, e := range events {
		byUID[e.UID] = e
	}

	lista := byUID["101@www.moodle.udesc.br"]
	if want := time.Date(2026, 11, 3, 2, 59, 0, 0, time.UTC); !lista.Start.Equal(want) {
		t.Errorf("Start = %v, quer %v", lista.Start, want)
	}
	if lista.Description != "Resolver os exercícios 1 a 10, entregar em PDF." {
		t.Errorf("escape de vírgula não tratado: %q", lista.Description)
	}
	if len(lista.Categories) != 1 || lista.Categories[0] != "ALP0001-2026-2" {
		t.Errorf("Categories = %v", lista.Categories)
	}

	// Descrição longa vem "dobrada" em várias linhas no arquivo.
	if got := byUID["105@www.moodle.udesc.br"].Description; len(got) < 80 {
		t.Errorf("line folding não foi desfeito: %q", got)
	}

	semana := byUID["106@www.moodle.udesc.br"]
	if !semana.AllDay {
		t.Error("evento VALUE=DATE deveria ser dia inteiro")
	}
	// 10/11 00:00 em São Paulo = 03:00 UTC
	if want := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC); !semana.Start.Equal(want) {
		t.Errorf("Start dia inteiro = %v, quer %v", semana.Start, want)
	}
}

func TestParseTimeFormats(t *testing.T) {
	cases := []struct {
		v      string
		params map[string][]string
		want   time.Time
		allDay bool
	}{
		{"20261103T025900Z", nil, time.Date(2026, 11, 3, 2, 59, 0, 0, time.UTC), false},
		{"20261102T235900", map[string][]string{"TZID": {"America/Sao_Paulo"}}, time.Date(2026, 11, 3, 2, 59, 0, 0, time.UTC), false},
		{"20261102T235900", nil, time.Date(2026, 11, 3, 2, 59, 0, 0, time.UTC), false},
		{"20261110", map[string][]string{"VALUE": {"DATE"}}, time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC), true},
	}
	for _, c := range cases {
		got, allDay, err := parseTime(c.v, c.params)
		if err != nil || !got.Equal(c.want) || allDay != c.allDay {
			t.Errorf("parseTime(%q) = %v,%v,%v; quer %v,%v", c.v, got, allDay, err, c.want, c.allDay)
		}
	}
	if _, _, err := parseTime("ontem", nil); err == nil {
		t.Error("esperava erro para data inválida")
	}
}
