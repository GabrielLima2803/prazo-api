package domain

import (
	"slices"
	"strings"
	"time"

	"prazo/internal/collections"
)

func (f Filter) Skip(d Deadline) bool {
	return f.Courses.Has(strings.ToUpper(d.CourseCode)) ||
		f.Courses.Has(strings.ToUpper(d.CourseName)) ||
		f.Types.Has(d.Type) ||
		(!f.Since.IsZero() && d.EndsAt.Before(f.Since)) ||
		(f.Titles != nil && f.Titles.MatchString(d.Title))
}

// Normalize aplica as regras de negócio ao que veio da fonte: agrupa os
// eventos de uma mesma atividade, descarta prazos de correção, aplica os
// filtros e calcula o hash. A saída é ordenada por data e SourceID.
func Normalize(in []Deadline, opts NormalizeOptions) []Deadline {
	out := make([]Deadline, 0, len(in))
	for _, d := range collapseActivities(in, opts.IncludeOpenings) {
		if d.Phase == PhaseGrading || opts.Filter.Skip(d) {
			continue
		}
		d.ContentHash = d.ComputeHash()
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b Deadline) int {
		if c := a.StartsAt.Compare(b.StartsAt); c != 0 {
			return c
		}
		return strings.Compare(a.SourceID, b.SourceID)
	})
	return out
}

// activityKey identifica a mesma atividade em eventos diferentes. A fonte
// iCal não expõe o id do módulo, então a chave é disciplina + título.
func activityKey(d Deadline) string {
	return d.CourseName + "\x1f" + strings.ToLower(strings.Join(strings.Fields(d.Title), " "))
}

type activity struct {
	phases collections.Set[Phase]
	openAt time.Time
}

// merged indica que Início e Término estão perto o bastante para virar um
// único evento com o horário real.
func (a activity) merged(closeAt time.Time) bool {
	gap := closeAt.Sub(a.openAt)
	return a.phases.Has(PhaseOpen) && a.phases.Has(PhaseClose) && gap > 0 && gap <= MaxMergedInterval
}

// collapseActivities mantém um evento por atividade:
//   - Início e Término próximos (MaxMergedInterval) viram um intervalo;
//   - havendo Término, o Início só fica com includeOpenings;
//   - "conclusão esperada" sai quando há Término ou prazo de entrega;
//   - Início sem Término (atividade ainda sem fechamento) fica.
func collapseActivities(in []Deadline, includeOpenings bool) []Deadline {
	acts := make(map[string]*activity, len(in))
	closeAt := make(map[string]time.Time, len(in))
	for _, d := range in {
		k := activityKey(d)
		a := acts[k]
		if a == nil {
			a = &activity{phases: collections.Set[Phase]{}}
			acts[k] = a
		}
		a.phases.Add(d.Phase)
		switch d.Phase {
		case PhaseOpen:
			a.openAt = d.StartsAt
		case PhaseClose:
			closeAt[k] = d.StartsAt
		}
	}

	out := make([]Deadline, 0, len(in))
	for _, d := range in {
		k := activityKey(d)
		a := acts[k]
		switch {
		case d.Phase == PhaseOpen && a.merged(closeAt[k]):
			continue
		case d.Phase == PhaseOpen && a.phases.Has(PhaseClose) && !includeOpenings:
			continue
		case d.Phase == PhaseExpect && (a.phases.Has(PhaseClose) || a.phases.Has(PhaseDue)):
			continue
		case d.Phase == PhaseClose && a.merged(d.StartsAt):
			d.EndsAt = d.StartsAt
			d.StartsAt = a.openAt
		}
		out = append(out, d)
	}
	return out
}
