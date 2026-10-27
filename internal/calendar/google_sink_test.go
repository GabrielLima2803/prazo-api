package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"
)

// fakeAPI imita os endpoints da Calendar API usados pelo Sink.
type fakeAPI struct {
	mu        sync.Mutex
	calendars map[string]bool
	events    map[string]map[string]any
	calls     []string
	failNext  []int // status devolvidos antes de processar as próximas chamadas
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{calendars: map[string]bool{}, events: map[string]map[string]any{}}
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
	f.calls = append(f.calls, r.Method+" "+path)

	if len(f.failNext) > 0 {
		code := f.failNext[0]
		f.failNext = f.failNext[1:]
		reason := "backendError"
		if code == http.StatusForbidden {
			reason = "rateLimitExceeded"
		}
		writeError(w, code, reason)
		return
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case r.Method == http.MethodPost && path == "/calendars":
		id := fmt.Sprintf("cal%d@group.calendar.google.com", len(f.calendars)+1)
		f.calendars[id] = true
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "calendars":
		if !f.calendars[parts[1]] {
			writeError(w, http.StatusNotFound, "notFound")
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": parts[1]})
	case len(parts) >= 3 && parts[2] == "events":
		f.handleEvent(w, r, parts)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeAPI) handleEvent(w http.ResponseWriter, r *http.Request, parts []string) {
	cal := parts[1]
	if !f.calendars[cal] {
		writeError(w, http.StatusNotFound, "notFound")
		return
	}
	var body map[string]any
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
	}
	switch {
	case r.Method == http.MethodPost:
		id := body["id"].(string)
		if _, ok := f.events[id]; ok {
			writeError(w, http.StatusConflict, "duplicate")
			return
		}
		f.events[id] = body
		json.NewEncoder(w).Encode(body)
	case r.Method == http.MethodPut:
		f.events[parts[3]] = body
		json.NewEncoder(w).Encode(body)
	case r.Method == http.MethodDelete:
		if _, ok := f.events[parts[3]]; !ok {
			writeError(w, http.StatusGone, "deleted")
			return
		}
		delete(f.events, parts[3])
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeError(w http.ResponseWriter, code int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":"%s","errors":[{"reason":"%s"}]}}`, code, reason, reason)
}

func newSink(t *testing.T, api *fakeAPI) *GoogleSink {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	s, err := NewGoogleSink(context.Background(), srv.Client(), option.WithEndpoint(srv.URL+"/calendar/v3/"))
	if err != nil {
		t.Fatal(err)
	}
	s.sleep = func(context.Context, time.Duration) error { return nil }
	return s
}

func event() Event {
	start := time.Date(2026, 10, 22, 2, 29, 0, 0, time.UTC)
	return Event{
		ID: EventID("1@moodle"), Summary: "[CDI] Entrega: T9", Start: start, End: start.Add(30 * time.Minute),
		TimeZone: "America/Sao_Paulo", ColorID: "3", Reminders: []time.Duration{24 * time.Hour},
		Link: "https://moodle/x", Properties: map[string]string{PropSourceID: "1@moodle"},
	}
}

func TestEnsureCalendar(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	ctx := context.Background()

	id, err := s.EnsureCalendar(ctx, "", Spec{Summary: "Prazo"})
	if err != nil || id == "" {
		t.Fatalf("criar: %q, %v", id, err)
	}
	if again, err := s.EnsureCalendar(ctx, id, Spec{}); err != nil || again != id {
		t.Fatalf("reaproveitar: %q, %v", again, err)
	}
	if recreated, err := s.EnsureCalendar(ctx, "apagada", Spec{}); err != nil || recreated == id || recreated == "apagada" {
		t.Fatalf("recriar agenda apagada: %q, %v", recreated, err)
	}
}

func TestUpsertCreatesThenUpdates(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	ctx := context.Background()
	cal, _ := s.EnsureCalendar(ctx, "", Spec{})

	if o, err := s.Upsert(ctx, cal, event()); err != nil || o != Created {
		t.Fatalf("primeiro upsert: %v, %v", o, err)
	}
	if o, err := s.Upsert(ctx, cal, event()); err != nil || o != Updated {
		t.Fatalf("segundo upsert: %v, %v", o, err)
	}
	if len(api.events) != 1 {
		t.Fatalf("eventos = %d; quer 1 (idempotente)", len(api.events))
	}

	got := api.events[event().ID]
	rem := got["reminders"].(map[string]any)
	if rem["useDefault"] != false || got["transparency"] != "transparent" || got["colorId"] != "3" {
		t.Errorf("evento enviado = %v", got)
	}
	if start := got["start"].(map[string]any); start["dateTime"] != "2026-10-22T02:29:00Z" || start["timeZone"] != "America/Sao_Paulo" {
		t.Errorf("start = %v", start)
	}
	if props := got["extendedProperties"].(map[string]any)["private"].(map[string]any); props["source_id"] != "1@moodle" {
		t.Errorf("extendedProperties = %v", props)
	}
}

func TestNoRemindersIsSentExplicitly(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	ctx := context.Background()
	cal, _ := s.EnsureCalendar(ctx, "", Spec{})
	e := event()
	e.Reminders = nil
	if _, err := s.Upsert(ctx, cal, e); err != nil {
		t.Fatal(err)
	}
	rem := api.events[e.ID]["reminders"].(map[string]any)
	if overrides, ok := rem["overrides"]; !ok || len(overrides.([]any)) != 0 {
		t.Fatalf("reminders = %v; quer overrides vazio explícito", rem)
	}
}

func TestRetriesRateLimit(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	ctx := context.Background()
	cal, _ := s.EnsureCalendar(ctx, "", Spec{})

	api.failNext = []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable}
	if o, err := s.Upsert(ctx, cal, event()); err != nil || o != Created {
		t.Fatalf("upsert após rate limit: %v, %v", o, err)
	}

	api.failNext = []int{http.StatusBadRequest}
	if _, err := s.Upsert(ctx, cal, event()); err == nil || statusCode(err) != http.StatusBadRequest {
		t.Fatalf("400 não deveria ser repetido: %v", err)
	}
}

func TestUnauthorizedIsAuthError(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	api.failNext = []int{http.StatusUnauthorized}
	_, err := s.EnsureCalendar(context.Background(), "", Spec{})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpsertIntoDeletedCalendar(t *testing.T) {
	s := newSink(t, newFakeAPI())
	if _, err := s.Upsert(context.Background(), "sumiu", event()); !errors.Is(err, ErrCalendarGone) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	api := newFakeAPI()
	s := newSink(t, api)
	ctx := context.Background()
	cal, _ := s.EnsureCalendar(ctx, "", Spec{})
	s.Upsert(ctx, cal, event())
	for range 2 {
		if err := s.Delete(ctx, cal, event().ID); err != nil {
			t.Fatal(err)
		}
	}
}
