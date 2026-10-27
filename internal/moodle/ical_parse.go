package moodle

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// Horários sem fuso explícito são interpretados no fuso da UDESC. O Brasil
// não tem horário de verão desde 2019, então UTC-3 fixo é um fallback seguro.
var floatingTZ = loadLocation("America/Sao_Paulo", time.FixedZone("-03", -3*60*60))

func loadLocation(name string, fallback *time.Location) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return fallback
}

// ParseICal usa golang-ical para line folding, parâmetros e escapes de TEXT. As
// datas são interpretadas aqui porque a lib usaria time.Local para horários
// flutuantes, deixando o resultado dependente da máquina.
func ParseICal(r io.Reader) (Feed, error) {
	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return Feed{}, fmt.Errorf("parse ical: %w", err)
	}
	var feed Feed
	for _, ve := range cal.Events() {
		ev, err := convert(ve)
		if err != nil {
			feed.Invalid = append(feed.Invalid, err)
			continue
		}
		feed.Events = append(feed.Events, ev)
	}
	return feed, nil
}

func convert(ve *ics.VEvent) (ICalEvent, error) {
	ev := ICalEvent{
		UID:         prop(ve, ics.ComponentPropertyUniqueId),
		Summary:     strings.TrimSpace(prop(ve, ics.ComponentPropertySummary)),
		Description: strings.TrimSpace(prop(ve, ics.ComponentPropertyDescription)),
		Location:    prop(ve, ics.ComponentPropertyLocation),
		URL:         prop(ve, ics.ComponentPropertyUrl),
	}
	if ev.UID == "" {
		return ICalEvent{}, fmt.Errorf("%w: %q", ErrMissingUID, ev.Summary)
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
		return ICalEvent{}, fmt.Errorf("%w: %s", ErrMissingDTStart, ev.UID)
	}
	var err error
	if ev.Start, ev.AllDay, err = parseTime(start.Value, start.ICalParameters); err != nil {
		return ICalEvent{}, fmt.Errorf("%s: DTSTART: %w", ev.UID, err)
	}
	ev.End = ev.Start
	if end := ve.GetProperty(ics.ComponentPropertyDtEnd); end != nil {
		if ev.End, _, err = parseTime(end.Value, end.ICalParameters); err != nil {
			return ICalEvent{}, fmt.Errorf("%s: DTEND: %w", ev.UID, err)
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

// parseTime aceita UTC (20240310T025900Z), local com ou sem TZID
// (20240310T235900) e dia inteiro (20240310 / VALUE=DATE).
func parseTime(v string, params map[string][]string) (t time.Time, allDay bool, err error) {
	loc := floatingTZ
	if tz := params["TZID"]; len(tz) == 1 {
		loc = loadLocation(tz[0], floatingTZ)
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
