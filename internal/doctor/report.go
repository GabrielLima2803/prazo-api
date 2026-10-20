// Package doctor analisa a exportação do Moodle para calibrar as regras de
// normalização. Não tem efeito colateral: só lê e resume.
package doctor

import (
	"cmp"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"prazo/internal/domain"
	"prazo/internal/source/ical"
)

// Counter é um multiconjunto simples: conta ocorrências e devolve o top-N.
type Counter map[string]int

func (c Counter) Add(k string) { c[k]++ }

type Entry struct {
	Key   string
	Count int
}

// Top devolve as n chaves mais frequentes (empate desfeito pela chave, para
// saída determinística). n <= 0 devolve todas.
func (c Counter) Top(n int) []Entry {
	out := make([]Entry, 0, len(c))
	for k, v := range c {
		out = append(out, Entry{k, v})
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Key, b.Key))
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

type Report struct {
	Total, Skipped  int
	SkippedSamples  []string
	DuplicateUIDs   []string
	UIDShapes       Counter // UID com dígitos trocados por "#"
	UIDSamples      []string
	Props           Counter // propriedades iCal presentes
	Categories      Counter // CATEGORIES (shortname da disciplina no Moodle)
	NoCategory      int
	Rules           Counter // regra de título que casou
	Types           Counter
	Phases          Counter
	Unmatched       []string // títulos sem template conhecido
	AllDay, ZeroDur int
	First, Last     time.Time
	Upcoming        []ical.Event // próximos eventos, ordenados
	upcomingWindow  time.Duration
}

var digits = regexp.MustCompile(`\d+`)

// Analyze monta o relatório. now e window definem a amostra de "próximos".
func Analyze(events []ical.Event, skipped []error, now time.Time, window time.Duration) *Report {
	r := &Report{
		Total:          len(events),
		Skipped:        len(skipped),
		UIDShapes:      Counter{},
		Props:          Counter{},
		Categories:     Counter{},
		Rules:          Counter{},
		Types:          Counter{},
		Phases:         Counter{},
		upcomingWindow: window,
	}
	for i, err := range skipped {
		if i == 5 {
			break
		}
		r.SkippedSamples = append(r.SkippedSamples, err.Error())
	}

	seen := make(map[string]struct{}, len(events)) // conjunto para achar UID duplicado
	unmatched := map[string]struct{}{}
	for _, e := range events {
		if _, dup := seen[e.UID]; dup {
			r.DuplicateUIDs = append(r.DuplicateUIDs, e.UID)
		}
		seen[e.UID] = struct{}{}

		shape := digits.ReplaceAllString(e.UID, "#")
		if r.UIDShapes[shape] == 0 && len(r.UIDSamples) < 3 {
			r.UIDSamples = append(r.UIDSamples, e.UID)
		}
		r.UIDShapes.Add(shape)

		for _, p := range e.Props {
			r.Props.Add(p)
		}
		if len(e.Categories) == 0 {
			r.NoCategory++
		}
		for _, c := range e.Categories {
			r.Categories.Add(c)
		}

		cl := domain.ClassifyTitle(e.Summary)
		rule := cl.Rule
		if rule == "" {
			rule = "(nenhuma)"
			if _, ok := unmatched[e.Summary]; !ok && len(unmatched) < 15 {
				unmatched[e.Summary] = struct{}{}
				r.Unmatched = append(r.Unmatched, e.Summary)
			}
		}
		r.Rules.Add(rule)
		r.Types.Add(string(cl.Type))
		r.Phases.Add(cmp.Or(string(cl.Phase), "(nenhuma)"))

		if e.AllDay {
			r.AllDay++
		}
		if e.End.Equal(e.Start) {
			r.ZeroDur++
		}
		if r.First.IsZero() || e.Start.Before(r.First) {
			r.First = e.Start
		}
		if e.Start.After(r.Last) {
			r.Last = e.Start
		}
		if !e.Start.Before(now) && e.Start.Before(now.Add(window)) {
			r.Upcoming = append(r.Upcoming, e)
		}
	}
	slices.SortFunc(r.Upcoming, func(a, b ical.Event) int { return a.Start.Compare(b.Start) })
	return r
}

// Print escreve o relatório legível. loc é usado só para exibir datas.
func (r *Report) Print(w io.Writer, loc *time.Location, maxUpcoming int) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	section := func(t string) { p("\n== %s ==\n", t) }
	list := func(c Counter, n int) {
		for _, e := range c.Top(n) {
			p("  %5d  %s\n", e.Count, e.Key)
		}
	}
	fmtT := func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.In(loc).Format("02/01/2006 15:04 MST")
	}

	section("iCal")
	p("  eventos válidos:   %d\n", r.Total)
	p("  eventos ignorados: %d\n", r.Skipped)
	for _, s := range r.SkippedSamples {
		p("    - %s\n", s)
	}
	p("  dia inteiro:       %d\n", r.AllDay)
	p("  duração zero:      %d (DTEND == DTSTART, típico de prazo)\n", r.ZeroDur)
	p("  intervalo:         %s → %s\n", fmtT(r.First), fmtT(r.Last))

	section("UIDs")
	p("  formatos (dígitos → #):\n")
	list(r.UIDShapes, 5)
	p("  exemplos: %s\n", strings.Join(r.UIDSamples, ", "))
	if len(r.DuplicateUIDs) > 0 {
		p("  ⚠ UIDs duplicados (%d): %s\n", len(r.DuplicateUIDs), strings.Join(r.DuplicateUIDs[:min(5, len(r.DuplicateUIDs))], ", "))
	} else {
		p("  ✓ nenhum UID duplicado (dá para usar como SourceID estável)\n")
	}

	section("Propriedades iCal presentes (eventos que têm)")
	list(r.Props, 0)

	section("Disciplinas (CATEGORIES)")
	list(r.Categories, 0)
	if r.NoCategory > 0 {
		p("  ⚠ %d evento(s) sem CATEGORIES\n", r.NoCategory)
	}

	section("Regras de título")
	list(r.Rules, 0)
	p("  tipos:\n")
	list(r.Types, 0)
	p("  fases:\n")
	list(r.Phases, 0)
	if len(r.Unmatched) > 0 {
		p("  títulos sem template (amostra):\n")
		for _, s := range r.Unmatched {
			p("    - %s\n", s)
		}
	}

	section(fmt.Sprintf("Próximos %d dias", int(r.upcomingWindow.Hours()/24)))
	if len(r.Upcoming) == 0 {
		p("  (nenhum)\n")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  QUANDO\tDISCIPLINA\tTIPO\tFASE\tTÍTULO")
	for i, e := range r.Upcoming {
		if maxUpcoming > 0 && i == maxUpcoming {
			fmt.Fprintf(tw, "  … +%d\t\t\t\t\n", len(r.Upcoming)-i)
			break
		}
		cl := domain.ClassifyTitle(e.Summary)
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			e.Start.In(loc).Format("Mon 02/01 15:04"), strings.Join(e.Categories, ","), cl.Type, cl.Phase, cl.Title)
	}
	_ = tw.Flush()
}
