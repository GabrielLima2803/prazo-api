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

// paletteSize é o número de cores de evento do Google Agenda (colorId 1–11).
const paletteSize = 11

// ColorID dá a cada disciplina uma cor estável entre execuções.
func ColorID(courseCode string) string {
	if courseCode == "" {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToUpper(courseCode)))
	return strconv.Itoa(int(h.Sum32()%paletteSize) + 1)
}

const day = 24 * time.Hour

var reminders = map[domain.DeadlineType][]time.Duration{
	domain.TypeAssign:      {3 * day, day, 2 * time.Hour},
	domain.TypeExam:        {7 * day, day},
	domain.TypeQuiz:        {day, 2 * time.Hour},
	domain.TypeForum:       {day},
	domain.TypeCourseEvent: {day},
	domain.TypeOther:       {day},
}

// Reminders some quando o prazo já foi entregue.
func Reminders(d domain.Deadline) []time.Duration {
	if d.Submitted {
		return nil
	}
	return reminders[d.Type]
}

// FromDeadline monta o evento: intervalos reais são mantidos; prazos pontuais
// viram um bloco de InstantDuration que termina no prazo.
func FromDeadline(d domain.Deadline, timeZone string) Event {
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
		ColorID:     ColorID(d.CourseCode),
		Reminders:   Reminders(d),
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
