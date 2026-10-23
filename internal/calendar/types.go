// Package calendar traduz prazos em eventos de agenda e define o destino
// (Sink). Não conhece o Google: a implementação fica em calendar/google.
package calendar

import (
	"context"
	"time"
)

// Spec descreve a agenda dedicada que o Prazo cria e mantém.
type Spec struct {
	Summary     string
	Description string
	TimeZone    string // IANA, ex.: America/Sao_Paulo
}

// Event é o evento já pronto para o destino, independente de provedor.
type Event struct {
	ID          string
	Summary     string
	Description string
	Link        string
	Start       time.Time // UTC
	End         time.Time // UTC
	TimeZone    string
	ColorID     string // "" = cor da agenda
	Reminders   []time.Duration
	Busy        bool              // ocupa a agenda (provas) ou não (prazos de entrega)
	Properties  map[string]string // metadados privados: source_id, content_hash
}

type Outcome int

const (
	Created Outcome = iota + 1
	Updated
)

func (o Outcome) String() string {
	switch o {
	case Created:
		return "criado"
	case Updated:
		return "atualizado"
	}
	return "desconhecido"
}

// Sink é o destino dos eventos. Implementações devem ser idempotentes: o
// mesmo Event aplicado duas vezes resulta num único evento.
type Sink interface {
	// EnsureCalendar devolve o id da agenda: knownID se ela ainda existir,
	// ou uma agenda nova criada a partir de spec.
	EnsureCalendar(ctx context.Context, knownID string, spec Spec) (string, error)
	Upsert(ctx context.Context, calendarID string, e Event) (Outcome, error)
	Delete(ctx context.Context, calendarID, eventID string) error
}
