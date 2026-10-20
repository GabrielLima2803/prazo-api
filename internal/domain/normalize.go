package domain

import (
	"slices"
	"strings"

	"prazo/internal/collections"
)

func (f Filter) Skip(d Deadline) bool {
	return f.Courses.Has(strings.ToUpper(d.CourseCode)) ||
		f.Courses.Has(strings.ToUpper(d.CourseName)) ||
		f.Types.Has(d.Type) ||
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

// collapseActivities mantém um evento por atividade:
//   - havendo Término, o Início só fica com includeOpenings;
//   - "conclusão esperada" sai quando há Término ou prazo de entrega;
//   - Início sem Término (atividade ainda sem fechamento) fica.
func collapseActivities(in []Deadline, includeOpenings bool) []Deadline {
	phases := make(map[string]collections.Set[Phase], len(in))
	for _, d := range in {
		k := activityKey(d)
		if phases[k] == nil {
			phases[k] = collections.Set[Phase]{}
		}
		phases[k].Add(d.Phase)
	}

	out := make([]Deadline, 0, len(in))
	for _, d := range in {
		has := phases[activityKey(d)]
		switch {
		case d.Phase == PhaseOpen && has.Has(PhaseClose) && !includeOpenings:
			continue
		case d.Phase == PhaseExpect && (has.Has(PhaseClose) || has.Has(PhaseDue)):
			continue
		}
		out = append(out, d)
	}
	return out
}
