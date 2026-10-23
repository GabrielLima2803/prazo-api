// Package domain contém as entidades e regras de negócio do Prazo. Não
// conhece Moodle, Google nem banco de dados.
package domain

import (
	"regexp"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
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

// DeadlineTypes lista os tipos na ordem de exibição.
var DeadlineTypes = []DeadlineType{TypeAssign, TypeExam, TypeQuiz, TypeForum, TypeCourseEvent, TypeOther}

var deadlineTypes = collections.NewSet(DeadlineTypes...)

// Phase é o momento da atividade que o evento representa.
type Phase string

const (
	PhaseNone    Phase = ""
	PhaseDue     Phase = "due"
	PhaseOpen    Phase = "open"
	PhaseClose   Phase = "close"
	PhaseGrading Phase = "grading" // prazo de correção, só relevante para professores
	PhaseExpect  Phase = "expect"  // "conclusão esperada" (completion tracking)
)

type Deadline struct {
	SourceID    string
	CourseCode  string
	CourseName  string
	Type        DeadlineType
	Phase       Phase
	Title       string
	Description string
	URL         string
	StartsAt    time.Time // UTC
	EndsAt      time.Time // UTC; igual a StartsAt quando o prazo é um instante
	Submitted   bool
	ContentHash string
}

// Filter descreve o que o usuário não quer na agenda.
type Filter struct {
	Courses collections.Set[string] // siglas ou nomes, em maiúsculas
	Types   collections.Set[DeadlineType]
	Titles  *regexp.Regexp
	Since   time.Time // prazos que terminam antes disso ficam de fora; zero = sem corte
}

// MaxMergedInterval é a maior distância entre Início e Término para que os
// dois virem um único evento com o horário real (ex.: prova online das 19h às
// 21h). Acima disso, a atividade fica aberta por dias e só o Término importa.
const MaxMergedInterval = 12 * time.Hour

type NormalizeOptions struct {
	Filter          Filter
	IncludeOpenings bool
}
