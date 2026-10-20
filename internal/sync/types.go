// Package sync é o caso de uso de sincronização: fetch → normalize → diff → apply.
package sync

import (
	"time"

	"prazo/internal/domain"
	"prazo/internal/source"
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
