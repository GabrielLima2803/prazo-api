package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"prazo/internal/domain"
)

type fakeSource struct {
	deadlines []domain.Deadline
	err       error
}

func (f fakeSource) Fetch(context.Context) ([]domain.Deadline, error) { return f.deadlines, f.err }

var now = time.Date(2026, 10, 20, 22, 0, 0, 0, time.UTC)

func clock() time.Time { return now }

func deadline(id string, phase domain.Phase, at time.Time) domain.Deadline {
	return domain.Deadline{SourceID: id, CourseName: "CDI", Title: "T9", Type: domain.TypeAssign, Phase: phase, StartsAt: at, EndsAt: at}
}

func TestPlanNormalizes(t *testing.T) {
	src := fakeSource{deadlines: []domain.Deadline{
		deadline("2", domain.PhaseClose, now.Add(48*time.Hour)),
		deadline("1", domain.PhaseOpen, now),
	}}
	plan, err := NewService(src, domain.NormalizeOptions{}, WithClock(clock)).Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Fetched != 2 || len(plan.Deadlines) != 1 || plan.Deadlines[0].SourceID != "2" || plan.Deadlines[0].ContentHash == "" {
		t.Fatalf("plan = %+v", plan)
	}
	if !plan.At.Equal(now) {
		t.Errorf("At = %v", plan.At)
	}
}

func TestPlanPropagatesSourceError(t *testing.T) {
	cause := errors.New("moodle fora do ar")
	_, err := NewService(fakeSource{err: cause}, domain.NormalizeOptions{}).Plan(context.Background())
	if !errors.Is(err, cause) {
		t.Fatalf("esperava o erro da fonte, veio %v", err)
	}
}

func TestUpcoming(t *testing.T) {
	plan := Plan{At: now, Deadlines: []domain.Deadline{
		deadline("passado", domain.PhaseDue, now.Add(-time.Hour)),
		deadline("agora", domain.PhaseDue, now),
		deadline("dentro", domain.PhaseDue, now.Add(29*24*time.Hour)),
		deadline("fora", domain.PhaseDue, now.Add(30*24*time.Hour)),
	}}
	got := plan.Upcoming(30 * 24 * time.Hour)
	if len(got) != 2 || got[0].SourceID != "agora" || got[1].SourceID != "dentro" {
		t.Fatalf("Upcoming = %+v", got)
	}
}
