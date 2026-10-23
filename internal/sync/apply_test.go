package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/calendartest"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	v, ok := m[k]
	if !ok {
		return nil, store.ErrNotFound
	}
	return v, nil
}

func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }

func (m memKV) Delete(_ context.Context, k string) error { delete(m, k); return nil }

func target(sink calendar.Sink, kv memKV) Target {
	return Target{Sink: sink, State: kv, Calendar: calendar.Spec{Summary: "Prazo — UDESC", TimeZone: "America/Sao_Paulo"}}
}

func threeDeadlines() fakeSource {
	return fakeSource{deadlines: []domain.Deadline{
		{SourceID: "a", Title: "A", Type: domain.TypeAssign, Phase: domain.PhaseDue, StartsAt: now, EndsAt: now},
		{SourceID: "b", Title: "B", Type: domain.TypeAssign, Phase: domain.PhaseDue, StartsAt: now.Add(time.Hour), EndsAt: now.Add(time.Hour)},
		{SourceID: "c", Title: "C", Type: domain.TypeExam, Phase: domain.PhaseDue, StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(2 * time.Hour)},
	}}
}

func TestSyncIsIdempotent(t *testing.T) {
	ctx := context.Background()
	sink, kv := calendartest.New(), memKV{}
	svc := NewService(threeDeadlines(), domain.NormalizeOptions{}, WithClock(clock))

	r, err := svc.Sync(ctx, target(sink, kv))
	if err != nil || r.Created != 3 || r.Updated != 0 {
		t.Fatalf("primeira execução: %+v, %v", r, err)
	}
	if string(kv[store.KeyGoogleCalendar]) != r.CalendarID {
		t.Fatal("id da agenda não foi guardado")
	}

	r2, err := svc.Sync(ctx, target(sink, kv))
	if err != nil || r2.Created != 0 || r2.Updated != 3 || r2.CalendarID != r.CalendarID {
		t.Fatalf("segunda execução: %+v, %v", r2, err)
	}
	if n := len(sink.Events(r.CalendarID)); n != 3 {
		t.Fatalf("eventos na agenda = %d; quer 3 (sem duplicar)", n)
	}
}

func TestSyncRecreatesDeletedCalendar(t *testing.T) {
	ctx := context.Background()
	sink, kv := calendartest.New(), memKV{}
	svc := NewService(threeDeadlines(), domain.NormalizeOptions{}, WithClock(clock))
	r, _ := svc.Sync(ctx, target(sink, kv))

	sink.DeleteCalendar(r.CalendarID)
	r2, err := svc.Sync(ctx, target(sink, kv))
	if err != nil || r2.CalendarID == r.CalendarID || r2.Created != 3 {
		t.Fatalf("após apagar a agenda: %+v, %v", r2, err)
	}
	if string(kv[store.KeyGoogleCalendar]) != r2.CalendarID {
		t.Fatal("novo id da agenda não foi guardado")
	}
}

func TestSyncContinuesAfterSingleFailure(t *testing.T) {
	sink := calendartest.New()
	sink.Fail = func(e calendar.Event) error {
		if e.Properties[calendar.PropSourceID] == "b" {
			return errors.New("400 inválido")
		}
		return nil
	}
	r, err := NewService(threeDeadlines(), domain.NormalizeOptions{}).Sync(context.Background(), target(sink, memKV{}))
	if err != nil || r.Created != 2 || r.Failed != 1 || r.Err == nil {
		t.Fatalf("report = %+v, err = %v", r, err)
	}
}

func TestSyncStopsOnAuthError(t *testing.T) {
	sink := calendartest.New()
	sink.Fail = func(calendar.Event) error { return calendar.ErrAuth }
	_, err := NewService(threeDeadlines(), domain.NormalizeOptions{}).Sync(context.Background(), target(sink, memKV{}))
	if !errors.Is(err, calendar.ErrAuth) || sink.Upserts != 1 {
		t.Fatalf("err = %v, upserts = %d; quer parar no primeiro", err, sink.Upserts)
	}
}

func TestSyncSourceErrorWritesNothing(t *testing.T) {
	sink := calendartest.New()
	_, err := NewService(fakeSource{err: errors.New("moodle fora")}, domain.NormalizeOptions{}).Sync(context.Background(), target(sink, memKV{}))
	if err == nil || sink.Upserts != 0 {
		t.Fatalf("err = %v, upserts = %d", err, sink.Upserts)
	}
}

func TestSyncAppliesPreferences(t *testing.T) {
	sink := calendartest.New()
	tg := target(sink, memKV{})
	tg.Prefs = prefs.Preferences{HiddenTypes: []domain.DeadlineType{domain.TypeExam}}
	r, err := NewService(threeDeadlines(), domain.NormalizeOptions{}).Sync(context.Background(), tg)
	if err != nil || r.Created != 2 || r.Hidden != 1 {
		t.Fatalf("report = %+v, err = %v", r, err)
	}
}
