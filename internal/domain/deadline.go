package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

func ParseDeadlineType(s string) (DeadlineType, error) {
	t := DeadlineType(strings.ToLower(strings.TrimSpace(s)))
	if !deadlineTypes.Has(t) {
		return "", fmt.Errorf("%w: %q", ErrUnknownType, s)
	}
	return t, nil
}

var typeLabels = map[DeadlineType]string{
	TypeAssign:      "Entrega",
	TypeQuiz:        "Questionário",
	TypeForum:       "Fórum",
	TypeExam:        "Prova",
	TypeCourseEvent: "Evento",
	TypeOther:       "Atividade",
}

func (t DeadlineType) Label() string {
	if l, ok := typeLabels[t]; ok {
		return l
	}
	return typeLabels[TypeOther]
}

func (d Deadline) Label() string {
	if d.Phase == PhaseOpen {
		return "Abertura"
	}
	return d.Type.Label()
}

// DisplayTitle monta "[SIGLA] Tipo: Título". O rótulo é omitido quando o
// título já começa com ele, e prazos entregues ganham o prefixo ✅.
func (d Deadline) DisplayTitle() string {
	var b strings.Builder
	if d.Submitted {
		b.WriteString("✅ ")
	}
	if d.CourseCode != "" {
		b.WriteString("[" + d.CourseCode + "] ")
	}
	if label := d.Label(); !hasPrefixFold(d.Title, label) {
		b.WriteString(label + ": ")
	}
	b.WriteString(d.Title)
	return b.String()
}

// ComputeHash cobre todo campo que, se mudar, exige atualizar o evento no
// destino. SourceID é a chave, não conteúdo, e fica de fora.
func (d Deadline) ComputeHash() string {
	h := sha256.New()
	for _, f := range [...]string{
		d.CourseCode, d.CourseName, string(d.Type), string(d.Phase), d.Title, d.Description, d.URL,
		strconv.FormatInt(d.StartsAt.Unix(), 10),
		strconv.FormatInt(d.EndsAt.Unix(), 10),
		strconv.FormatBool(d.Submitted),
	} {
		h.Write([]byte(f))
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsInstant indica um prazo sem duração (entrega até tal hora), em oposição
// a um intervalo real (prova das 19h às 21h).
func (d Deadline) IsInstant() bool { return !d.EndsAt.After(d.StartsAt) }

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
