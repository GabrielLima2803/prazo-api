package domain

import "testing"

func TestClassifyTitle(t *testing.T) {
	cases := []struct {
		in    string
		typ   DeadlineType
		phase Phase
		title string
	}{
		{"Lista 3 está marcado(a) para esta data", TypeAssign, PhaseDue, "Lista 3"},
		{"Lista 3 está com avaliação marcada para esta data", TypeAssign, PhaseGrading, "Lista 3"},
		{"Fórum de apresentação é devido", TypeForum, PhaseDue, "Fórum de apresentação"},
		{"Início de Prova 1 - Unidade 1", TypeExam, PhaseOpen, "Prova 1 - Unidade 1"},
		{"Término de Questionário Semanal 5", TypeQuiz, PhaseClose, "Questionário Semanal 5"},
		{"Início de Pesquisa de satisfação", TypeOther, PhaseOpen, "Pesquisa de satisfação"},
		{"Leitura do capítulo 4 deve estar concluído", TypeOther, PhaseExpect, "Leitura do capítulo 4"},
		{"Definição de grupo para MVP (Data limite)", TypeOther, PhaseDue, "Definição de grupo para MVP"},
		{"Lista 3 is due", TypeAssign, PhaseDue, "Lista 3"},
		{"Quiz 2 closes", TypeQuiz, PhaseClose, "Quiz 2"},
		{"Semana Acadêmica", TypeOther, PhaseNone, "Semana Acadêmica"},
		{"  Prova substitutiva  ", TypeExam, PhaseNone, "Prova substitutiva"},
		// Só o template, sem nome: não pode virar título vazio.
		{"Início de ", TypeOther, PhaseNone, "Início de"},
	}
	for _, c := range cases {
		got := ClassifyTitle(c.in)
		if got.Type != c.typ || got.Phase != c.phase || got.Title != c.title {
			t.Errorf("ClassifyTitle(%q) = %+v; quer type=%s phase=%q title=%q", c.in, got, c.typ, c.phase, c.title)
		}
	}
}
