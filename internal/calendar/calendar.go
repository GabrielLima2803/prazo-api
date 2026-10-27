package calendar

import (
	"context"
	"errors"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

var (
	// ErrAuth indica credencial revogada ou expirada: nenhuma escrita vai
	// funcionar até um novo "prazo auth".
	ErrAuth = errors.New("autorização do Google inválida; rode \"prazo auth\" de novo")
	// ErrCalendarGone indica que a agenda foi apagada pelo usuário.
	ErrCalendarGone = errors.New("agenda não existe mais")
)

// Spec descreve a agenda dedicada que o Prazo cria e mantém.
type Spec struct {
	Summary     string
	Description string
	TimeZone    string // IANA, ex.: America/Sao_Paulo
}

// Style são as personalizações do usuário sobre o padrão: cor por disciplina
// e lembretes por tipo. Mapas vazios ou nil mantêm o padrão.
type Style struct {
	Colors    map[string]string // sigla (maiúsculas) → colorId 1–11
	Reminders map[domain.DeadlineType][]time.Duration
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
