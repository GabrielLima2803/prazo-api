package moodle

import (
	"testing"

	"prazo/internal/domain"
)

func TestParseEventName(t *testing.T) {
	cases := []struct {
		in    string
		typ   domain.DeadlineType
		phase domain.Phase
		title string
	}{
		{"Lista 3 está marcado(a) para esta data", domain.TypeAssign, domain.PhaseDue, "Lista 3"},
		{"Lista 3 está com avaliação marcada para esta data", domain.TypeAssign, domain.PhaseGrading, "Lista 3"},
		{"Fórum de apresentação é devido", domain.TypeForum, domain.PhaseDue, "Fórum de apresentação"},
		{"Início de Prova 1 - Unidade 1", domain.TypeExam, domain.PhaseOpen, "Prova 1 - Unidade 1"},
		{"Término de Questionário Semanal 5", domain.TypeQuiz, domain.PhaseClose, "Questionário Semanal 5"},
		{"Início de Pesquisa de satisfação", domain.TypeOther, domain.PhaseOpen, "Pesquisa de satisfação"},
		{"Leitura do capítulo 4 deve estar concluído", domain.TypeOther, domain.PhaseExpect, "Leitura do capítulo 4"},
		{"Definição de grupo para MVP (Data limite)", domain.TypeOther, domain.PhaseDue, "Definição de grupo para MVP"},
		{"Lista 3 is due", domain.TypeAssign, domain.PhaseDue, "Lista 3"},
		{"Quiz 2 closes", domain.TypeQuiz, domain.PhaseClose, "Quiz 2"},
		{"Semana Acadêmica", domain.TypeOther, domain.PhaseNone, "Semana Acadêmica"},
		{"  Prova substitutiva  ", domain.TypeExam, domain.PhaseNone, "Prova substitutiva"},
		{"Início de ", domain.TypeOther, domain.PhaseNone, "Início de"},

		// Títulos reais do Moodle do CCT (2026-10).
		{"Exercício Slide submission is due", domain.TypeAssign, domain.PhaseDue, "Exercício Slide"},
		{"Término de TAREFA 9: Intervalo de convergência", domain.TypeAssign, domain.PhaseClose, "TAREFA 9: Intervalo de convergência"},
		{"TAREFA 4: Integrais Impróprias deve estar concluído", domain.TypeAssign, domain.PhaseExpect, "TAREFA 4: Integrais Impróprias"},
		{"Prova 1 está marcado(a) para esta data", domain.TypeExam, domain.PhaseDue, "Prova 1"},
		{"Prova de ALI - P1", domain.TypeExam, domain.PhaseNone, "Prova de ALI - P1"},
		{"UPLOAD da Tarefa-1  FILA DE PRIORIDADE está marcado(a) para esta data", domain.TypeAssign, domain.PhaseDue, "UPLOAD da Tarefa-1 FILA DE PRIORIDADE"},
	}
	for _, c := range cases {
		got := ParseEventName(c.in)
		if got.Type != c.typ || got.Phase != c.phase || got.Title != c.title {
			t.Errorf("ParseEventName(%q) = %+v; quer type=%s phase=%q title=%q", c.in, got, c.typ, c.phase, c.title)
		}
	}
}

func TestCourseCode(t *testing.T) {
	cases := map[string]string{
		"2026/2_CCT_CCI251-03A_POO0002": "POO",
		"2026/2_CCT_CCI251-03A_EDA1002": "EDA",
		"2026/2_CCT_CCI251-03U_CDI2002": "CDI",
		"ALP0001-2026-2":                "ALP",
		"alp":                           "ALP",
		"SITE":                          "SITE",
		"12345":                         "12345",
		"":                              "",
	}
	for in, want := range cases {
		if got := CourseCode(in); got != want {
			t.Errorf("CourseCode(%q) = %q; quer %q", in, got, want)
		}
	}
}
