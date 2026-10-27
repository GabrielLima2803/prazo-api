package sync

import (
	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

// Sync calcula o plano e leva o destino a ele:
//   - novo ou alterado → upsert; igual ao que já está lá → nada;
//   - oculto pelas preferências → apaga;
//   - sumiu do Moodle e ainda não passou → apaga, com proteção contra apagar
//     em massa; sumiu porque já passou → só esquece.
//
// Nada é apagado se a fonte falhou: Plan devolve erro antes. Uma falha num
// evento não interrompe os demais; autorização e cancelamento interrompem.
func (s *Service) Sync(ctx context.Context, t Target) (Report, error) {
	plan, err := s.Plan(ctx)
	if err != nil {
		return Report{}, err
	}
	r := Report{Plan: plan}
	if r.CalendarID, err = ensureCalendar(ctx, t); err != nil {
		return r, err
	}
	st, err := loadState(ctx, t.State, r.CalendarID)
	if err != nil {
		return r, err
	}
	// Salva o progresso mesmo se a execução parar no meio.
	defer func() {
		if err := saveState(ctx, t.State, st); err != nil {
			r.Err = errors.Join(r.Err, err)
		}
	}()

	style := t.Prefs.Style()
	inPlan := make(map[string]bool, len(plan.Deadlines))
	var errs []error
	for _, d := range plan.Deadlines {
		inPlan[d.SourceID] = true
		if t.Prefs.Hides(d) {
			r.Hidden++
			continue
		}
		ev := calendar.FromDeadline(d, t.Calendar.TimeZone, style)
		hash := ev.Hash()
		if prev, ok := st.Events[d.SourceID]; ok && prev.Hash == hash {
			r.Unchanged++
			continue
		}
		outcome, err := t.Sink.Upsert(ctx, r.CalendarID, ev)
		switch {
		case err == nil:
			st.Events[d.SourceID] = syncedEvent{EventID: ev.ID, Hash: hash, EndsAt: d.EndsAt}
			if outcome == calendar.Created {
				r.Created++
			} else {
				r.Updated++
			}
		case ctx.Err() != nil, errors.Is(err, calendar.ErrAuth), errors.Is(err, calendar.ErrCalendarGone):
			return r, err
		default:
			r.Failed++
			errs = append(errs, err)
		}
	}

	if err := s.removeStale(ctx, t, &r, st, plan, inPlan); err != nil {
		return r, err
	}
	r.Err = errors.Join(errs...)
	if r.Failed > 0 && r.Created+r.Updated+r.Unchanged == 0 {
		return r, fmt.Errorf("nenhum evento aplicado: %w", r.Err)
	}
	return r, nil
}

func (s *Service) removeStale(ctx context.Context, t Target, r *Report, st *state, plan Plan, inPlan map[string]bool) error {
	var hidden, gone []string
	for id, ev := range st.Events {
		switch {
		case inPlan[id] && t.Prefs.Hides(planDeadline(plan, id)):
			hidden = append(hidden, id)
		case inPlan[id]:
		case ev.EndsAt.Before(plan.At.Add(-pastGrace)):
			delete(st.Events, id) // saiu da janela da exportação por ser antigo
		default:
			gone = append(gone, id)
		}
	}
	guard := max(minDeleteGuard, int(math.Ceil(maxDeleteRatio*float64(len(st.Events)))))
	if len(gone) > guard {
		r.DeleteBlocked = len(gone)
		s.log.Warn("muitos prazos sumiram de uma vez do Moodle; nada foi apagado",
			"sumiram", len(gone), "limite", guard)
		gone = nil
	}
	for _, id := range append(hidden, gone...) {
		err := t.Sink.Delete(ctx, r.CalendarID, st.Events[id].EventID)
		switch {
		case err == nil:
			delete(st.Events, id)
			r.Deleted++
		case ctx.Err() != nil, errors.Is(err, calendar.ErrAuth):
			return err
		default:
			r.Failed++
			r.Err = errors.Join(r.Err, err)
		}
	}
	return nil
}

func planDeadline(p Plan, sourceID string) (d domain.Deadline) {
	for _, x := range p.Deadlines {
		if x.SourceID == sourceID {
			return x
		}
	}
	return d
}

func loadState(ctx context.Context, kv storage.KV, calendarID string) (*state, error) {
	st := &state{CalendarID: calendarID, Events: map[string]syncedEvent{}}
	b, err := kv.Get(ctx, storage.KeySyncState)
	if errors.Is(err, storage.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	var saved state
	if err := json.Unmarshal(b, &saved); err != nil {
		return nil, fmt.Errorf("estado da sincronização corrompido: %w", err)
	}
	// Agenda recriada (o usuário apagou a antiga): o estado não vale mais.
	if saved.CalendarID == calendarID && saved.Events != nil {
		st.Events = saved.Events
	}
	return st, nil
}

func saveState(ctx context.Context, kv storage.KV, st *state) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return kv.Put(ctx, storage.KeySyncState, b)
}

func ensureCalendar(ctx context.Context, t Target) (string, error) {
	known, err := t.State.Get(ctx, storage.KeyGoogleCalendar)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	id, err := t.Sink.EnsureCalendar(ctx, string(known), t.Calendar)
	if err != nil {
		return "", err
	}
	if id != string(known) {
		if err := t.State.Put(ctx, storage.KeyGoogleCalendar, []byte(id)); err != nil {
			return "", fmt.Errorf("guardar id da agenda %s: %w", id, err)
		}
	}
	return id, nil
}
