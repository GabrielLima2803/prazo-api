package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

// Sync calcula o plano e aplica todos os prazos no destino. Por enquanto não
// remove nada: sem estado local (Fase 3), não há como distinguir um prazo
// apagado no Moodle de um que só saiu da janela da exportação.
//
// Uma falha num evento não interrompe os demais. Erros de autorização e
// cancelamento interrompem, porque nenhuma escrita seguinte daria certo.
func (s *Service) Sync(ctx context.Context, t Target) (Report, error) {
	plan, err := s.Plan(ctx)
	if err != nil {
		return Report{}, err
	}
	r := Report{Plan: plan}
	if r.CalendarID, err = ensureCalendar(ctx, t); err != nil {
		return r, err
	}

	var errs []error
	for _, d := range plan.Deadlines {
		outcome, err := t.Sink.Upsert(ctx, r.CalendarID, calendar.FromDeadline(d, t.Calendar.TimeZone))
		switch {
		case err == nil && outcome == calendar.Created:
			r.Created++
		case err == nil:
			r.Updated++
		case ctx.Err() != nil, errors.Is(err, calendar.ErrAuth), errors.Is(err, calendar.ErrCalendarGone):
			return r, err
		default:
			r.Failed++
			errs = append(errs, err)
		}
	}
	r.Err = errors.Join(errs...)
	if r.Failed > 0 && r.Created+r.Updated == 0 {
		return r, fmt.Errorf("nenhum evento aplicado: %w", r.Err)
	}
	return r, nil
}

func ensureCalendar(ctx context.Context, t Target) (string, error) {
	known, err := t.State.Get(ctx, store.KeyGoogleCalendar)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	id, err := t.Sink.EnsureCalendar(ctx, string(known), t.Calendar)
	if err != nil {
		return "", err
	}
	if id != string(known) {
		if err := t.State.Put(ctx, store.KeyGoogleCalendar, []byte(id)); err != nil {
			return "", fmt.Errorf("guardar id da agenda %s: %w", id, err)
		}
	}
	return id, nil
}
