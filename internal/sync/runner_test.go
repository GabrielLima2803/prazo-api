package sync

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/calendartest"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// blockingSink segura o Upsert até release fechar.
type blockingSink struct {
	*calendartest.Fake
	started chan struct{}
	release chan struct{}
}

func (b blockingSink) Upsert(ctx context.Context, cal string, e calendar.Event) (calendar.Outcome, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-b.release
	return b.Fake.Upsert(ctx, cal, e)
}

func TestRunnerRejectsConcurrentRuns(t *testing.T) {
	sink := blockingSink{calendartest.New(), make(chan struct{}, 1), make(chan struct{})}
	svc := NewService(threeDeadlines(), domain.NormalizeOptions{}, WithClock(clock))
	r := NewRunner(svc, func(context.Context) (Target, error) { return target(sink, memKV{}), nil }, discard)

	done := make(chan error)
	go func() { _, err := r.RunNow(context.Background()); done <- err }()
	<-sink.started
	if _, err := r.RunNow(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("segunda execução: %v; quer ErrBusy", err)
	}
	close(sink.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	last, ok := r.Last()
	if !ok || last.Report.Created != 3 {
		t.Fatalf("Last = %+v", last)
	}
}

func TestRunnerKeepsErrorAndCachesPlan(t *testing.T) {
	src := &switchableSource{fakeSource: threeDeadlines()}
	svc := NewService(src, domain.NormalizeOptions{}, WithClock(clock))
	notAuthorized := errors.New("google não autorizado")
	r := NewRunner(svc, func(context.Context) (Target, error) { return Target{}, notAuthorized }, discard)

	if _, err := r.RunNow(context.Background()); !errors.Is(err, notAuthorized) {
		t.Fatalf("err = %v", err)
	}
	if last, _ := r.Last(); !errors.Is(last.Err, notAuthorized) {
		t.Fatalf("Last.Err = %v", last.Err)
	}

	p, err := r.Plan(context.Background(), time.Hour)
	if err != nil || len(p.Deadlines) != 3 {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	src.err = errors.New("moodle fora")
	if p, err := r.Plan(context.Background(), 0); err != nil || len(p.Deadlines) != 3 {
		t.Fatalf("com a fonte fora deveria usar o último plano: %+v, %v", p, err)
	}
}

type switchableSource struct {
	fakeSource
	err error
}

func (s *switchableSource) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.fakeSource.Fetch(ctx)
}
