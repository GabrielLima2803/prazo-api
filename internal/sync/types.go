// Package sync é o caso de uso de sincronização: fetch → normalize → diff → apply.
package sync

import (
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
	Plan       Plan
	CalendarID string
	Created    int
	Updated    int
	Hidden     int // ocultos pelas preferências; não enviados
	Failed     int
	Err        error // falhas individuais (errors.Join); a execução seguiu
}
