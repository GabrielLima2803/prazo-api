package domain

import (
	"regexp"
	"slices"
	"strings"
)

// Filter decide o que fica fora da agenda. Os conjuntos (map[string]struct{})
// dão busca O(1) por evento.
type Filter struct {
	Courses map[string]struct{} // siglas ou shortnames, em maiúsculas
	Types   map[DeadlineType]struct{}
	Titles  *regexp.Regexp // nil = não filtra por título
}

// Skip diz se o prazo deve ser ignorado.
func (f Filter) Skip(d Deadline) bool {
	if _, ok := f.Courses[strings.ToUpper(d.CourseCode)]; ok {
		return true
	}
	if _, ok := f.Courses[strings.ToUpper(d.CourseName)]; ok {
		return true
	}
	if _, ok := f.Types[d.Type]; ok {
		return true
	}
	return f.Titles != nil && f.Titles.MatchString(d.Title)
}

type NormalizeOptions struct {
	Filter          Filter
	IncludeOpenings bool // manter eventos "Início de X" quando houver "Término de X"
}

// Normalize aplica as regras de negócio sobre o que veio da fonte:
//  1. descarta prazos de correção (só fazem sentido para professores);
//  2. agrupa os eventos da mesma atividade (Início/Término/conclusão esperada)
//     e mantém só o prazo que importa;
//  3. aplica os filtros do usuário;
//  4. calcula o ContentHash.
//
// O resultado sai ordenado por data e depois por SourceID (determinístico).
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

// activityKey identifica "a mesma atividade" entre eventos diferentes: o
// Moodle não dá o id do módulo no iCal, então usamos disciplina + título.
func activityKey(d Deadline) string {
	return d.CourseName + "\x1f" + strings.ToLower(strings.Join(strings.Fields(d.Title), " "))
}

// collapseActivities resolve os eventos múltiplos de uma mesma atividade:
//   - com Término (close): fica o Término; Início só se includeOpenings;
//     "deve estar concluído" (expect) é descartado;
//   - com prazo de entrega (due): "expect" é descartado;
//   - Início sozinho (atividade ainda sem fechamento) é mantido.
func collapseActivities(in []Deadline, includeOpenings bool) []Deadline {
	phases := make(map[string]map[Phase]bool, len(in))
	for _, d := range in {
		k := activityKey(d)
		if phases[k] == nil {
			phases[k] = map[Phase]bool{}
		}
		phases[k][d.Phase] = true
	}

	out := make([]Deadline, 0, len(in))
	for _, d := range in {
		has := phases[activityKey(d)]
		switch d.Phase {
		case PhaseOpen:
			if has[PhaseClose] && !includeOpenings {
				continue
			}
		case PhaseExpect:
			if has[PhaseClose] || has[PhaseDue] {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}
