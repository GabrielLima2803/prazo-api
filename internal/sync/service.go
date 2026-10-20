package sync

import (
	"context"
	"fmt"
	"time"

	"prazo/internal/domain"
	"prazo/internal/source"
)

type Option func(*Service)

// WithClock troca o relógio (testes).
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

func NewService(src source.Source, opts domain.NormalizeOptions, options ...Option) *Service {
	s := &Service{source: src, opts: opts, now: time.Now}
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
