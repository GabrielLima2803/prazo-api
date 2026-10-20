package moodle

import (
	"regexp"
	"strings"

	"prazo/internal/domain"
)

// templates em ordem de prioridade. Fonte: pacotes de idioma do Moodle 4.4
// (pt_br e en) e títulos reais do Moodle do CCT.
var templates = []nameTemplate{
	{rule: "assign.grading.pt", suffix: " está com avaliação marcada para esta data", typ: domain.TypeAssign, phase: domain.PhaseGrading},
	{rule: "assign.due.pt", suffix: " está marcado(a) para esta data", typ: domain.TypeAssign, phase: domain.PhaseDue},
	{rule: "forum.due.pt", suffix: " é devido", typ: domain.TypeForum, phase: domain.PhaseDue},
	{rule: "completion.pt", suffix: " deve estar concluído", typ: domain.TypeOther, phase: domain.PhaseExpect},
	{rule: "groupselect.due.pt", suffix: " (Data limite)", typ: domain.TypeOther, phase: domain.PhaseDue},
	{rule: "open.pt", prefix: "Início de ", typ: domain.TypeOther, phase: domain.PhaseOpen},
	{rule: "close.pt", prefix: "Término de ", typ: domain.TypeOther, phase: domain.PhaseClose},
	{rule: "open.pt.activity", prefix: "Abrir atividade ", typ: domain.TypeOther, phase: domain.PhaseOpen},
	{rule: "close.pt.activity", prefix: "Fechar atividade ", typ: domain.TypeOther, phase: domain.PhaseClose},

	{rule: "assign.grading.en", suffix: " is due to be graded", typ: domain.TypeAssign, phase: domain.PhaseGrading},
	{rule: "submission.due.en", suffix: " submission is due", typ: domain.TypeAssign, phase: domain.PhaseDue},
	{rule: "due.en", suffix: " is due", typ: domain.TypeAssign, phase: domain.PhaseDue},
	{rule: "open.en", suffix: " opens", typ: domain.TypeOther, phase: domain.PhaseOpen},
	{rule: "close.en", suffix: " closes", typ: domain.TypeOther, phase: domain.PhaseClose},
	{rule: "completion.en", suffix: " should be completed", typ: domain.TypeOther, phase: domain.PhaseExpect},
}

// Os templates de abertura/fechamento servem para vários módulos (quiz,
// feedback, escolha...), então o tipo é deduzido pelo nome da atividade.
var (
	examWords   = regexp.MustCompile(`(?i)\b(prova|exame|avalia[çc][ãa]o)\b`)
	quizWords   = regexp.MustCompile(`(?i)\b(question[áa]rio|quiz|teste|simulado)\b`)
	assignWords = regexp.MustCompile(`(?i)\b(tarefa|lista|trabalho|atividade|exerc[íi]cio|entrega|upload)\b`)
)

func ParseEventName(name string) EventName {
	s := strings.Join(strings.Fields(name), " ")
	for _, t := range templates {
		if !hasPrefixFold(s, t.prefix) || !hasSuffixFold(s, t.suffix) {
			continue
		}
		title := strings.TrimSpace(s[len(t.prefix) : len(s)-len(t.suffix)])
		if title == "" {
			continue
		}
		return EventName{Type: refineType(t.typ, title), Phase: t.phase, Title: title, Rule: t.rule}
	}
	return EventName{Type: guessType(s), Phase: domain.PhaseNone, Title: s}
}

func refineType(typ domain.DeadlineType, title string) domain.DeadlineType {
	switch {
	case typ == domain.TypeOther:
		return guessType(title)
	case typ == domain.TypeAssign && examWords.MatchString(title):
		// Professores criam provas como tarefa ("Prova 1"); os lembretes devem ser de prova.
		return domain.TypeExam
	default:
		return typ
	}
}

func guessType(title string) domain.DeadlineType {
	switch {
	case examWords.MatchString(title):
		return domain.TypeExam
	case quizWords.MatchString(title):
		return domain.TypeQuiz
	case assignWords.MatchString(title):
		return domain.TypeAssign
	default:
		return domain.TypeOther
	}
}

// Se o corte cair no meio de um caractere multibyte, EqualFold devolve false.
func hasPrefixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p)
}

func hasSuffixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[len(s)-len(p):], p)
}
