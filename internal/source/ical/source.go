package ical

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"prazo/internal/domain"
)

// Source adapta o Fetcher à interface source.Source.
type Source struct {
	Fetcher *Fetcher
	Log     *slog.Logger
}

func (s *Source) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	events, skipped, err := s.Fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range skipped {
		s.Log.Warn("evento iCal ignorado", "err", e)
	}
	base, err := baseURL(s.Fetcher.URL)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Deadline, 0, len(events))
	for _, e := range events {
		out = append(out, ToDeadline(e, base))
	}
	return out, nil
}

// baseURL extrai "https://host" da URL de exportação (sem o authtoken).
func baseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("ical: URL inválida")
	}
	return u.Scheme + "://" + u.Host, nil
}

// Módulos que aparecem em links da descrição mas não são "a atividade"
// (slides, páginas, pastas que o professor citou no enunciado).
var nonActivityModules = map[string]struct{}{
	"resource": {}, "url": {}, "page": {}, "folder": {}, "label": {}, "book": {},
}

var (
	activityLink = regexp.MustCompile(`https?://[^\s\]]+/mod/(\w+)/view\.php\?id=\d+`)
	linkMarker   = regexp.MustCompile(`\s*\[\d+\]`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
)

// ToDeadline converte um VEVENT do Moodle em Deadline (sem hash; ver Normalize).
func ToDeadline(e Event, base string) domain.Deadline {
	cl := domain.ClassifyTitle(e.Summary)
	d := domain.Deadline{
		SourceID: e.UID,
		Type:     cl.Type,
		Phase:    cl.Phase,
		Title:    cl.Title,
		StartsAt: e.Start.UTC(),
		EndsAt:   e.End.UTC(),
	}
	if len(e.Categories) > 0 {
		d.CourseName = e.Categories[0]
		d.CourseCode = domain.CourseCode(d.CourseName)
		if d.Phase == domain.PhaseNone && d.Type == domain.TypeOther {
			d.Type = domain.TypeCourseEvent
		}
	}

	d.URL = activityURL(e.Description)
	if d.URL == "" {
		// O iCal não traz o link da atividade; o dia no calendário é o melhor
		// atalho que dá para montar só com o que temos.
		d.URL = base + "/calendar/view.php?view=day&time=" + strconv.FormatInt(e.Start.Unix(), 10)
	}
	d.Description = cleanDescription(e.Description, cl.Title)
	return d
}

func activityURL(desc string) string {
	for _, m := range activityLink.FindAllStringSubmatch(desc, -1) {
		if _, skip := nonActivityModules[m[1]]; !skip {
			return m[0]
		}
	}
	return ""
}

// cleanDescription tira o rodapé "Links: ------ [1] url" que o Moodle gera ao
// converter HTML em texto, e os marcadores [n] no corpo.
func cleanDescription(desc, title string) string {
	if i := strings.Index(desc, "\nLinks:\n------"); i >= 0 {
		desc = desc[:i]
	}
	desc = linkMarker.ReplaceAllString(desc, "")
	desc = blankLines.ReplaceAllString(strings.TrimSpace(desc), "\n\n")
	if strings.EqualFold(desc, title) {
		return "" // o "corpo" era só o nome autolinkado da atividade
	}
	return desc
}
