package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"
)

type Service struct {
	source moodle.Source
	opts   domain.NormalizeOptions
	now    func() time.Time
	log    *slog.Logger
}

// Plan é o estado desejado calculado a partir da fonte.
type Plan struct {
	Deadlines []domain.Deadline // normalizados e ordenados por data
	Fetched   int               // itens brutos recebidos da fonte
	At        time.Time
}

// Target é o destino de uma sincronização.
type Target struct {
	Sink     calendar.Sink
	State    storage.KV // guarda o id da agenda entre execuções
	Calendar calendar.Spec
	Prefs    Preferences
}

type Report struct {
	Plan          Plan
	CalendarID    string
	Created       int
	Updated       int
	Unchanged     int // iguais ao que já está no destino; nada enviado
	Deleted       int
	DeleteBlocked int // remoções seguradas pela proteção contra apagar em massa
	Hidden        int // ocultos pelas preferências
	Failed        int
	Err           error // falhas individuais (errors.Join); a execução seguiu
}

// state é o que o Prazo já pôs na agenda, por SourceID. Permite enviar só o
// que mudou e saber o que apagar.
type state struct {
	CalendarID string                 `json:"calendarId"`
	Events     map[string]syncedEvent `json:"events"`
}

type syncedEvent struct {
	EventID string    `json:"eventId"`
	Hash    string    `json:"hash"`
	EndsAt  time.Time `json:"endsAt"`
}

// Proteção contra apagar em massa: se mais que isso sumir de uma vez do
// Moodle, algo deu errado na fonte (ex.: exportação parcial) e nada é apagado.
const (
	maxDeleteRatio = 0.3
	minDeleteGuard = 3
	// Prazos que já passaram somem da exportação do Moodle com o tempo; isso
	// não é motivo para apagar o evento da agenda.
	pastGrace = 24 * time.Hour
)

type Option func(*Service)

// WithClock troca o relógio (testes).
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLogger define onde vão os avisos (ex.: remoções seguradas).
func WithLogger(l *slog.Logger) Option {
	return func(s *Service) { s.log = l }
}

func NewService(src moodle.Source, opts domain.NormalizeOptions, options ...Option) *Service {
	s := &Service{source: src, opts: opts, now: time.Now, log: slog.New(slog.DiscardHandler)}
	for _, o := range options {
		o(s)
	}
	return s
}

func (s *Service) Plan(ctx context.Context) (Plan, error) {
	raw, err := s.source.Fetch(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("plan: %w", err)
	}
	return Plan{
		Deadlines: domain.Normalize(raw, s.opts),
		Fetched:   len(raw),
		At:        s.now().UTC(),
	}, nil
}

// Upcoming devolve os prazos em [p.At, p.At+window).
func (p Plan) Upcoming(window time.Duration) []domain.Deadline {
	end := p.At.Add(window)
	var out []domain.Deadline
	for _, d := range p.Deadlines {
		if !d.StartsAt.Before(p.At) && d.StartsAt.Before(end) {
			out = append(out, d)
		}
	}
	return out
}
