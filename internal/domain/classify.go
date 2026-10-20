package domain

import (
	"regexp"
	"strings"
)

type DeadlineType string

const (
	TypeAssign      DeadlineType = "assign"
	TypeQuiz        DeadlineType = "quiz"
	TypeForum       DeadlineType = "forum"
	TypeExam        DeadlineType = "exam"
	TypeCourseEvent DeadlineType = "course_event"
	TypeOther       DeadlineType = "other"
)

// Phase diz qual "momento" da atividade o evento representa.
type Phase string

const (
	PhaseDue     Phase = "due"     // prazo final de entrega
	PhaseOpen    Phase = "open"    // abertura (quiz, feedback...)
	PhaseClose   Phase = "close"   // fechamento
	PhaseGrading Phase = "grading" // prazo de correção (só para professores)
	PhaseExpect  Phase = "expect"  // conclusão esperada (completion)
	PhaseNone    Phase = ""        // evento comum, sem template
)

// Classification é o resultado de interpretar um SUMMARY do Moodle.
type Classification struct {
	Type  DeadlineType
	Phase Phase
	Title string // nome da atividade, sem o texto do template
	Rule  string // qual regra casou (diagnóstico); vazio se nenhuma
}

// titleRule descreve um template de nome de evento do Moodle. O Moodle grava o
// nome do evento já traduzido no idioma do site na hora da criação, então
// precisamos reconhecer pt_br (idioma da UDESC) e en (fallback).
type titleRule struct {
	name   string
	prefix string
	suffix string
	typ    DeadlineType
	phase  Phase
}

// titleRules em ordem de prioridade. Fontes: pacotes de idioma do Moodle 4.4
// (mod_assign calendardue/calendargradingdue, mod_quiz quizeventopens/closes,
// mod_forum calendardue, core_completion completionexpectedfor).
var titleRules = []titleRule{
	{name: "assign.grading.pt", suffix: " está com avaliação marcada para esta data", typ: TypeAssign, phase: PhaseGrading},
	{name: "assign.due.pt", suffix: " está marcado(a) para esta data", typ: TypeAssign, phase: PhaseDue},
	{name: "forum.due.pt", suffix: " é devido", typ: TypeForum, phase: PhaseDue},
	{name: "completion.pt", suffix: " deve estar concluído", typ: TypeOther, phase: PhaseExpect},
	// mod_groupselect (plugin instalado no Moodle do CCT).
	{name: "groupselect.due.pt", suffix: " (Data limite)", typ: TypeOther, phase: PhaseDue},
	{name: "open.pt", prefix: "Início de ", typ: TypeOther, phase: PhaseOpen},
	{name: "close.pt", prefix: "Término de ", typ: TypeOther, phase: PhaseClose},
	{name: "open.pt.activity", prefix: "Abrir atividade ", typ: TypeOther, phase: PhaseOpen},
	{name: "close.pt.activity", prefix: "Fechar atividade ", typ: TypeOther, phase: PhaseClose},

	{name: "assign.grading.en", suffix: " is due to be graded", typ: TypeAssign, phase: PhaseGrading},
	{name: "due.en", suffix: " is due", typ: TypeAssign, phase: PhaseDue},
	{name: "open.en", suffix: " opens", typ: TypeOther, phase: PhaseOpen},
	{name: "close.en", suffix: " closes", typ: TypeOther, phase: PhaseClose},
	{name: "completion.en", suffix: " should be completed", typ: TypeOther, phase: PhaseExpect},
}

// Palavras no nome da atividade que indicam o tipo quando o template sozinho
// é ambíguo ("Início de X" serve para quiz, feedback, escolha, base de dados...).
var (
	examWords = regexp.MustCompile(`(?i)\b(prova|exame|avalia[çc][ãa]o)\b`)
	quizWords = regexp.MustCompile(`(?i)\b(question[áa]rio|quiz|teste|simulado)\b`)
)

// ClassifyTitle interpreta o SUMMARY de um evento do Moodle.
func ClassifyTitle(summary string) Classification {
	s := strings.TrimSpace(summary)
	for _, r := range titleRules {
		if r.prefix != "" && !hasPrefixFold(s, r.prefix) {
			continue
		}
		if r.suffix != "" && !hasSuffixFold(s, r.suffix) {
			continue
		}
		title := strings.TrimSpace(s[len(r.prefix) : len(s)-len(r.suffix)])
		if title == "" {
			continue
		}
		typ := r.typ
		if typ == TypeOther {
			typ = guessType(title)
		}
		return Classification{Type: typ, Phase: r.phase, Title: title, Rule: r.name}
	}
	return Classification{Type: guessType(s), Phase: PhaseNone, Title: s}
}

func guessType(title string) DeadlineType {
	switch {
	case examWords.MatchString(title):
		return TypeExam
	case quizWords.MatchString(title):
		return TypeQuiz
	default:
		return TypeOther
	}
}

// Comparação sem diferenciar maiúsculas. Se o corte cair no meio de um
// caractere multibyte, EqualFold devolve false, que é o resultado correto.
func hasPrefixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p)
}

func hasSuffixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[len(s)-len(p):], p)
}
