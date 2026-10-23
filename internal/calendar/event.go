package calendar

import (
	"crypto/sha256"
	"encoding/base32"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

// InstantDuration é a duração do evento de um prazo pontual. O evento termina
// no prazo, para que o horário exibido na agenda seja o limite real.
const InstantDuration = 30 * time.Minute

const (
	PropSourceID    = "source_id"
	PropContentHash = "content_hash"
)

// O Google aceita ids de evento com [a-v0-9] e 5 a 1024 caracteres:
// exatamente o alfabeto base32hex em minúsculas.
const maxEventID = 1024

var idEncoding = base32.NewEncoding("0123456789abcdefghijklmnopqrstuv").WithPadding(base32.NoPadding)

// EventID é determinístico: o mesmo prazo sempre gera o mesmo id, o que torna
// a criação idempotente mesmo se o processo cair no meio da sincronização.
func EventID(sourceID string) string {
	raw := []byte("prazo:" + sourceID)
	if idEncoding.EncodedLen(len(raw)) > maxEventID {
		sum := sha256.Sum256(raw)
		raw = sum[:]
	}
	return idEncoding.EncodeToString(raw)
}

// Palette são as cores de evento do Google Agenda (colorId → hex), para que
// o app mostre exatamente a cor que o evento tem na agenda.
var Palette = map[string]string{
	"1": "#7986CB", "2": "#33B679", "3": "#8E24AA", "4": "#E67C73",
	"5": "#F6BF26", "6": "#F4511E", "7": "#039BE5", "8": "#616161",
	"9": "#3F51B5", "10": "#0B8043", "11": "#D50000",
}

// ColorID dá a cada disciplina uma cor estável entre execuções.
func ColorID(courseCode string) string {
	if courseCode == "" {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToUpper(courseCode)))
	return strconv.Itoa(int(h.Sum32()%uint32(len(Palette))) + 1)
}

// ColorFor aplica a escolha do usuário, se houver, sobre a cor padrão.
func (s Style) ColorFor(courseCode string) string {
	if c, ok := s.Colors[strings.ToUpper(courseCode)]; ok && courseCode != "" {
		return c
	}
	return ColorID(courseCode)
}

const day = 24 * time.Hour

// DefaultReminders são os lembretes de cada tipo quando o usuário não
// personalizou.
var DefaultReminders = map[domain.DeadlineType][]time.Duration{
	domain.TypeAssign:      {3 * day, day, 2 * time.Hour},
	domain.TypeExam:        {7 * day, day},
	domain.TypeQuiz:        {day, 2 * time.Hour},
	domain.TypeForum:       {day},
	domain.TypeCourseEvent: {day},
	domain.TypeOther:       {day},
}

// RemindersFor some quando o prazo já foi entregue. Uma lista vazia
// definida pelo usuário desliga os lembretes daquele tipo.
func (s Style) RemindersFor(d domain.Deadline) []time.Duration {
	if d.Submitted {
		return nil
	}
	if r, ok := s.Reminders[d.Type]; ok {
		return r
	}
	return DefaultReminders[d.Type]
}

// FromDeadline monta o evento: intervalos reais são mantidos; prazos pontuais
// viram um bloco de InstantDuration que termina no prazo.
func FromDeadline(d domain.Deadline, timeZone string, style Style) Event {
	start, end := d.StartsAt, d.EndsAt
	if d.IsInstant() {
		end = d.StartsAt
		start = end.Add(-InstantDuration)
	}
	return Event{
		ID:          EventID(d.SourceID),
		Summary:     d.DisplayTitle(),
		Description: description(d),
		Link:        d.URL,
		Start:       start.UTC(),
		End:         end.UTC(),
		TimeZone:    timeZone,
		ColorID:     style.ColorFor(d.CourseCode),
		Reminders:   style.RemindersFor(d),
		Busy:        d.Type == domain.TypeExam && !d.IsInstant(),
		Properties: map[string]string{
			PropSourceID:    d.SourceID,
			PropContentHash: d.ContentHash,
		},
	}
}

func description(d domain.Deadline) string {
	var parts []string
	if d.Description != "" {
		parts = append(parts, d.Description)
	}
	var meta []string
	if d.URL != "" {
		meta = append(meta, "Abrir no Moodle: "+d.URL)
	}
	if d.CourseName != "" {
		meta = append(meta, "Disciplina: "+d.CourseName)
	}
	if len(meta) > 0 {
		parts = append(parts, strings.Join(meta, "\n"))
	}
	parts = append(parts, "— sincronizado pelo Prazo")
	return strings.Join(parts, "\n\n")
}
