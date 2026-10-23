// Package calendartest tem um calendar.Sink em memória para testes.
package calendartest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"prazo/internal/calendar"
)

type Fake struct {
	mu        sync.Mutex
	calendars map[string]map[string]calendar.Event
	nextID    int

	// Fail, se definido, é consultado antes de cada Upsert.
	Fail func(e calendar.Event) error

	Upserts int
}

func New() *Fake {
	return &Fake{calendars: map[string]map[string]calendar.Event{}}
}

func (f *Fake) EnsureCalendar(_ context.Context, knownID string, _ calendar.Spec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.calendars[knownID]; ok {
		return knownID, nil
	}
	f.nextID++
	id := fmt.Sprintf("cal%d", f.nextID)
	f.calendars[id] = map[string]calendar.Event{}
	return id, nil
}

func (f *Fake) Upsert(_ context.Context, calendarID string, e calendar.Event) (calendar.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Upserts++
	if f.Fail != nil {
		if err := f.Fail(e); err != nil {
			return 0, err
		}
	}
	events, ok := f.calendars[calendarID]
	if !ok {
		return 0, calendar.ErrCalendarGone
	}
	_, exists := events[e.ID]
	events[e.ID] = e
	if exists {
		return calendar.Updated, nil
	}
	return calendar.Created, nil
}

func (f *Fake) Delete(_ context.Context, calendarID, eventID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.calendars[calendarID], eventID)
	return nil
}

// DeleteCalendar simula o usuário apagando a agenda no Google.
func (f *Fake) DeleteCalendar(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.calendars, id)
}

// Events devolve os eventos da agenda ordenados por id.
func (f *Fake) Events(calendarID string) []calendar.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	events := f.calendars[calendarID]
	out := make([]calendar.Event, 0, len(events))
	for _, id := range slices.Sorted(maps.Keys(events)) {
		out = append(out, events[id])
	}
	return out
}
