package moodle

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

func NewICalSource(client *ICalClient, log *slog.Logger) *ICalSource {
	return &ICalSource{client: client, baseURL: client.BaseURL(), log: log}
}

func (s *ICalSource) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	feed, err := s.client.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range feed.Invalid {
		s.log.Warn("evento iCal ignorado", "err", e)
	}
	out := make([]domain.Deadline, 0, len(feed.Events))
	for _, e := range feed.Events {
		out = append(out, ToDeadline(e, s.baseURL))
	}
	return out, nil
}

// Módulos citados na descrição que não são a atividade em si (slides,
// páginas, pastas do enunciado).
var nonActivityModules = collections.NewSet("resource", "url", "page", "folder", "label", "book")

var (
	activityLink = regexp.MustCompile(`https?://[^\s\]]+/mod/(\w+)/view\.php\?id=\d+`)
	linkMarker   = regexp.MustCompile(`\s*\[\d+\]`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
)

// ToDeadline converte um evento do Moodle em Deadline, sem hash
// (ver domain.Normalize).
func ToDeadline(e ICalEvent, baseURL string) domain.Deadline {
	name := ParseEventName(e.Summary)
	d := domain.Deadline{
		SourceID: e.UID,
		Type:     name.Type,
		Phase:    name.Phase,
		Title:    name.Title,
		StartsAt: e.Start,
		EndsAt:   e.End,
	}
	if len(e.Categories) > 0 {
		d.CourseName = e.Categories[0]
		d.CourseCode = CourseCode(d.CourseName)
		if d.Phase == domain.PhaseNone && d.Type == domain.TypeOther {
			d.Type = domain.TypeCourseEvent
		}
	}

	d.URL = activityURL(e.Description)
	if d.URL == "" {
		// O iCal não traz o link da atividade; o dia no calendário é o melhor atalho disponível.
		d.URL = baseURL + "/calendar/view.php?view=day&time=" + strconv.FormatInt(e.Start.Unix(), 10)
	}
	d.Description = cleanDescription(e.Description, name.Title)
	return d
}

func activityURL(desc string) string {
	for _, m := range activityLink.FindAllStringSubmatch(desc, -1) {
		if !nonActivityModules.Has(m[1]) {
			return m[0]
		}
	}
	return ""
}

// cleanDescription remove o rodapé "Links: ------ [n] url" e os marcadores
// [n] que o Moodle gera ao converter HTML em texto.
func cleanDescription(desc, title string) string {
	if i := strings.Index(desc, "\nLinks:\n------"); i >= 0 {
		desc = desc[:i]
	}
	desc = linkMarker.ReplaceAllString(desc, "")
	desc = blankLines.ReplaceAllString(strings.TrimSpace(desc), "\n\n")
	if strings.EqualFold(desc, title) {
		return "" // era só o nome da atividade, autolinkado pelo filtro do Moodle
	}
	return desc
}
