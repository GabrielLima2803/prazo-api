// Package ical lê a exportação iCalendar do Moodle.
//
// Biblioteca: github.com/arran4/golang-ical. Escolhida por ser a mais usada
// no ecossistema Go, mantida ativamente, sem dependências transitivas e
// tolerante a arquivos "quase RFC 5545" (o Moodle às vezes gera linhas
// longas e escapes inconsistentes). Ela cuida de line folding, parâmetros
// e escapes de TEXT; a interpretação de datas fazemos aqui, porque o
// helper da lib usa time.Local para horários "flutuantes", o que tornaria o
// resultado dependente da máquina.
package ical

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// Event é um VEVENT cru, sem nenhuma regra de negócio aplicada.
type Event struct {
	UID          string
	Summary      string
	Description  string
	Categories   []string
	Location     string
	URL          string
	Start        time.Time // UTC
	End          time.Time // UTC; igual a Start quando não há DTEND
	AllDay       bool
	LastModified time.Time
	Props        []string // nomes das propriedades presentes (para diagnóstico)
}

// floatingTZ é usado para horários sem fuso explícito. O Moodle exporta em
// UTC ("Z"), mas se um dia vier "flutuante", o fuso da UDESC é o correto.
// Sem horário de verão desde 2019, então UTC-3 fixo é um fallback seguro.
var floatingTZ = loadOr("America/Sao_Paulo", time.FixedZone("-03", -3*60*60))

func loadOr(name string, fallback *time.Location) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return fallback
}

// Parse lê um calendário iCal e devolve os eventos. Eventos individuais
// malformados (sem UID ou sem data) são devolvidos em skipped, não abortam.
func Parse(r io.Reader) (events []Event, skipped []error, err error) {
	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ical: %w", err)
	}
	for _, ve := range cal.Events() {
		ev, err := convert(ve)
		if err != nil {
			skipped = append(skipped, err)
			continue
		}
		events = append(events, ev)
	}
	return events, skipped, nil
}

func convert(ve *ics.VEvent) (Event, error) {
	ev := Event{
		UID:         prop(ve, ics.ComponentPropertyUniqueId),
		Summary:     strings.TrimSpace(prop(ve, ics.ComponentPropertySummary)),
		Description: strings.TrimSpace(prop(ve, ics.ComponentPropertyDescription)),
		Location:    prop(ve, ics.ComponentPropertyLocation),
		URL:         prop(ve, ics.ComponentPropertyUrl),
	}
	if ev.UID == "" {
		return Event{}, fmt.Errorf("evento %q sem UID", ev.Summary)
	}

	for _, p := range ve.Properties {
		ev.Props = append(ev.Props, p.IANAToken)
		if p.IANAToken == string(ics.ComponentPropertyCategories) {
			for c := range strings.SplitSeq(p.Value, ",") {
				if c = strings.TrimSpace(c); c != "" {
					ev.Categories = append(ev.Categories, c)
				}
			}
		}
	}
	slices.Sort(ev.Props)
	ev.Props = slices.Compact(ev.Props)

	start := ve.GetProperty(ics.ComponentPropertyDtStart)
	if start == nil {
		return Event{}, fmt.Errorf("evento %s sem DTSTART", ev.UID)
	}
	var err error
	if ev.Start, ev.AllDay, err = parseTime(start.Value, start.ICalParameters); err != nil {
		return Event{}, fmt.Errorf("evento %s: DTSTART: %w", ev.UID, err)
	}
	ev.End = ev.Start
	if end := ve.GetProperty(ics.ComponentPropertyDtEnd); end != nil {
		if ev.End, _, err = parseTime(end.Value, end.ICalParameters); err != nil {
			return Event{}, fmt.Errorf("evento %s: DTEND: %w", ev.UID, err)
		}
	}
	if lm := ve.GetProperty(ics.ComponentPropertyLastModified); lm != nil {
		ev.LastModified, _, _ = parseTime(lm.Value, lm.ICalParameters)
	}
	return ev, nil
}

func prop(ve *ics.VEvent, p ics.ComponentProperty) string {
	if v := ve.GetProperty(p); v != nil {
		return v.Value
	}
	return ""
}

// parseTime entende os três formatos que aparecem na prática:
// 20240310T025900Z (UTC), 20240310T235900 com ou sem TZID, e 20240310 (dia inteiro).
func parseTime(v string, params map[string][]string) (t time.Time, allDay bool, err error) {
	loc := floatingTZ
	if tz := params["TZID"]; len(tz) == 1 {
		if l, err := time.LoadLocation(tz[0]); err == nil {
			loc = l
		}
	}
	isDate := len(params["VALUE"]) == 1 && params["VALUE"][0] == "DATE"

	switch {
	case isDate || len(v) == 8:
		t, err = time.ParseInLocation("20060102", v, loc)
		allDay = true
	case strings.HasSuffix(v, "Z"):
		t, err = time.Parse("20060102T150405Z", v)
	default:
		t, err = time.ParseInLocation("20060102T150405", v, loc)
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("data inválida %q: %w", v, err)
	}
	return t.UTC(), allDay, nil
}
