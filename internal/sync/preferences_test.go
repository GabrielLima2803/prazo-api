package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

func TestSaveLoadNormalizes(t *testing.T) {
	ctx := context.Background()
	s := NewPreferenceStore(memKV{})
	if p, err := s.Load(ctx); err != nil || len(p.HiddenCourses) != 0 {
		t.Fatalf("vazio: %+v, %v", p, err)
	}
	saved, err := s.Save(ctx, Preferences{
		HiddenCourses: []string{"poo", "POO", " cdi "},
		CourseColors:  map[string]string{"eda": "3"},
		Reminders:     map[domain.DeadlineType][]int{domain.TypeAssign: {60, 1440, 60}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load(ctx)
	if len(got.HiddenCourses) != 2 || got.HiddenCourses[0] != "CDI" || got.CourseColors["EDA"] != "3" {
		t.Errorf("normalizado = %+v", got)
	}
	if r := got.Reminders[domain.TypeAssign]; len(r) != 2 || r[0] != 1440 {
		t.Errorf("lembretes = %v; quer [1440 60]", r)
	}
	if len(saved.HiddenCourses) != 2 {
		t.Errorf("Save deveria devolver o normalizado: %+v", saved)
	}
}

func TestValidate(t *testing.T) {
	bad := Preferences{
		HiddenTypes:  []domain.DeadlineType{"prova"},
		CourseColors: map[string]string{"POO": "12"},
		Reminders:    map[domain.DeadlineType][]int{domain.TypeQuiz: {0, 1, 2, 3, 4, 5}},
	}
	if _, err := NewPreferenceStore(memKV{}).Save(context.Background(), bad); !errors.Is(err, ErrInvalidPreferences) {
		t.Fatalf("err = %v", err)
	}
}

func TestHidesAndStyle(t *testing.T) {
	p := Preferences{
		HiddenCourses: []string{"POO"},
		HiddenTypes:   []domain.DeadlineType{domain.TypeForum},
		Reminders:     map[domain.DeadlineType][]int{domain.TypeAssign: {90}},
	}
	if !p.Hides(domain.Deadline{CourseCode: "poo"}) || !p.Hides(domain.Deadline{Type: domain.TypeForum}) || p.Hides(domain.Deadline{CourseCode: "CDI"}) {
		t.Error("Hides errado")
	}
	if r := p.Style().Reminders[domain.TypeAssign]; len(r) != 1 || r[0] != 90*time.Minute {
		t.Errorf("Style = %v", r)
	}
}

func TestEmptyPreferencesEncodeAsEmptyCollections(t *testing.T) {
	p, err := NewPreferenceStore(memKV{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if want := `{"hiddenCourses":[],"hiddenTypes":[],"courseColors":{},"reminders":{}}`; string(b) != want {
		t.Fatalf("JSON = %s; quer %s", b, want)
	}
}
