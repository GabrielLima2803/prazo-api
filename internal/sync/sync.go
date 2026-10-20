// Package sync é o caso de uso: fetch → normalize → diff → apply.
// Na Fase 1 só existem fetch e normalize (dry-run).
package sync

import (
	"context"
	"fmt"

	"prazo/internal/domain"
	"prazo/internal/source"
)

type Service struct {
	Source source.Source
	Opts   domain.NormalizeOptions
}

// Plan busca a fonte e devolve os prazos normalizados, mais quantos itens
// brutos vieram (para o relatório).
func (s *Service) Plan(ctx context.Context) (deadlines []domain.Deadline, fetched int, err error) {
	raw, err := s.Source.Fetch(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("plan: %w", err)
	}
	return domain.Normalize(raw, s.Opts), len(raw), nil
}
