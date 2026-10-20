package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Deadline é um prazo/evento já normalizado, independente de Moodle e Google.
type Deadline struct {
	SourceID    string // ID estável vindo do Moodle (UID do iCal ou id do evento)
	CourseCode  string // sigla, ex.: POO
	CourseName  string // shortname ou nome completo da disciplina
	Type        DeadlineType
	Phase       Phase
	Title       string // nome da atividade, sem o texto do template do Moodle
	Description string
	URL         string    // link para a atividade (ou para o dia no calendário)
	StartsAt    time.Time // UTC
	EndsAt      time.Time // UTC
	Submitted   bool
	ContentHash string // hash dos campos relevantes, usado no diff
}

// ComputeHash calcula o hash de tudo que, se mudar, exige atualizar o evento
// no Google. SourceID fica de fora: é a chave, não conteúdo.
func (d Deadline) ComputeHash() string {
	h := sha256.New()
	for _, f := range []string{
		d.CourseCode, d.CourseName, string(d.Type), string(d.Phase), d.Title, d.Description, d.URL,
		strconv.FormatInt(d.StartsAt.UTC().Unix(), 10),
		strconv.FormatInt(d.EndsAt.UTC().Unix(), 10),
		strconv.FormatBool(d.Submitted),
	} {
		h.Write([]byte(f))
		h.Write([]byte{0x1f}) // separador: evita que "ab"+"c" colida com "a"+"bc"
	}
	return hex.EncodeToString(h.Sum(nil))
}

var typeLabels = map[DeadlineType]string{
	TypeAssign:      "Entrega",
	TypeQuiz:        "Questionário",
	TypeForum:       "Fórum",
	TypeExam:        "Prova",
	TypeCourseEvent: "Evento",
	TypeOther:       "Atividade",
}

// TypeLabel é o rótulo em português usado no título.
func TypeLabel(t DeadlineType, p Phase) string {
	if p == PhaseOpen {
		return "Abertura"
	}
	if l, ok := typeLabels[t]; ok {
		return l
	}
	return typeLabels[TypeOther]
}

// RenderTitle monta o título padronizado: "[SIGLA] Tipo: Título".
// Se o título já começa com o rótulo ("Entrega Etapa 3"), o rótulo não é
// repetido. Entregues ganham o prefixo ✅.
func RenderTitle(d Deadline) string {
	var b strings.Builder
	if d.Submitted {
		b.WriteString("✅ ")
	}
	if d.CourseCode != "" {
		b.WriteString("[" + d.CourseCode + "] ")
	}
	label := TypeLabel(d.Type, d.Phase)
	if !hasPrefixFold(d.Title, label) {
		b.WriteString(label + ": ")
	}
	b.WriteString(d.Title)
	return b.String()
}
