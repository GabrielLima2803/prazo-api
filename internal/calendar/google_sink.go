package calendar

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
)

// GoogleScope é o mínimo necessário: só agendas criadas pelo próprio Prazo. A
// agenda principal do usuário fica fora de alcance.
const GoogleScope = gcal.CalendarAppCreatedScope

type GoogleSink struct {
	svc        *gcal.Service
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	sleep      func(context.Context, time.Duration) error
}

// NewGoogleSink usa hc para todas as chamadas; em produção é o cliente do oauth2,
// que injeta e renova o access token.
func NewGoogleSink(ctx context.Context, hc *http.Client, opts ...option.ClientOption) (*GoogleSink, error) {
	svc, err := gcal.NewService(ctx, append([]option.ClientOption{option.WithHTTPClient(hc)}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("google calendar: %w", err)
	}
	return &GoogleSink{svc: svc, maxRetries: 5, baseDelay: time.Second, maxDelay: 32 * time.Second, sleep: httpx.Sleep}, nil
}

func (s *GoogleSink) EnsureCalendar(ctx context.Context, knownID string, spec Spec) (string, error) {
	if knownID != "" {
		err := s.call(ctx, func() error {
			_, err := s.svc.Calendars.Get(knownID).Context(ctx).Do()
			return err
		})
		switch code := statusCode(err); {
		case err == nil:
			return knownID, nil
		case code != http.StatusNotFound && code != http.StatusGone:
			return "", fmt.Errorf("consultar agenda: %w", err)
		}
	}
	var created *gcal.Calendar
	err := s.call(ctx, func() (err error) {
		created, err = s.svc.Calendars.Insert(&gcal.Calendar{
			Summary:     spec.Summary,
			Description: spec.Description,
			TimeZone:    spec.TimeZone,
		}).Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", fmt.Errorf("criar agenda: %w", err)
	}
	return created.Id, nil
}

// Upsert tenta criar com o id determinístico; se já existir (409), atualiza.
// Atualizar também restaura um evento que o usuário apagou na agenda.
func (s *GoogleSink) Upsert(ctx context.Context, calendarID string, e Event) (Outcome, error) {
	ev := toGoogle(e)
	err := s.call(ctx, func() error {
		_, err := s.svc.Events.Insert(calendarID, ev).Context(ctx).Do()
		return err
	})
	switch statusCode(err) {
	case 0:
		if err != nil {
			return 0, err
		}
		return Created, nil
	case http.StatusConflict:
	case http.StatusNotFound:
		return 0, fmt.Errorf("evento %s: %w", e.ID, ErrCalendarGone)
	default:
		return 0, fmt.Errorf("criar evento %s: %w", e.ID, err)
	}

	err = s.call(ctx, func() error {
		_, err := s.svc.Events.Update(calendarID, e.ID, ev).Context(ctx).Do()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("atualizar evento %s: %w", e.ID, err)
	}
	return Updated, nil
}

func (s *GoogleSink) Delete(ctx context.Context, calendarID, eventID string) error {
	err := s.call(ctx, func() error {
		return s.svc.Events.Delete(calendarID, eventID).Context(ctx).Do()
	})
	if code := statusCode(err); code == http.StatusNotFound || code == http.StatusGone {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apagar evento %s: %w", eventID, err)
	}
	return nil
}

func (s *GoogleSink) call(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = classify(fn()); err == nil || !retryable(err) || attempt >= s.maxRetries {
			return err
		}
		if err := s.sleep(ctx, httpx.Backoff(attempt+1, s.baseDelay, s.maxDelay)); err != nil {
			return err
		}
	}
}

func toGoogle(e Event) *gcal.Event {
	overrides := make([]*gcal.EventReminder, 0, len(e.Reminders))
	for _, r := range e.Reminders {
		overrides = append(overrides, &gcal.EventReminder{Method: "popup", Minutes: int64(r / time.Minute)})
	}
	ev := &gcal.Event{
		Id:          e.ID,
		Status:      "confirmed",
		Summary:     e.Summary,
		Description: e.Description,
		ColorId:     e.ColorID,
		Start:       &gcal.EventDateTime{DateTime: e.Start.Format(time.RFC3339), TimeZone: e.TimeZone},
		End:         &gcal.EventDateTime{DateTime: e.End.Format(time.RFC3339), TimeZone: e.TimeZone},
		Reminders: &gcal.EventReminders{
			UseDefault:      false,
			Overrides:       overrides,
			ForceSendFields: []string{"UseDefault", "Overrides"}, // sem isso, "sem lembretes" seria omitido do JSON
		},
		ExtendedProperties: &gcal.EventExtendedProperties{Private: e.Properties},
		Transparency:       "transparent",
	}
	if e.Busy {
		ev.Transparency = "opaque"
	}
	if e.Link != "" {
		ev.Source = &gcal.EventSource{Title: "Moodle", Url: e.Link}
	}
	return ev
}
