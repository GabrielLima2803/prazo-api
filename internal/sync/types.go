// Package sync é o caso de uso de sincronização: fetch → normalize → diff → apply.
package sync

import (
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/source"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type Service struct {
	source source.Source
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
	State    store.KV // guarda o id da agenda entre execuções
	Calendar calendar.Spec
	Prefs    prefs.Preferences
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
