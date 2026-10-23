package prefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

func NewStore(kv store.KV) *Store { return &Store{kv: kv} }

// Load devolve preferências vazias (tudo no padrão) se nada foi salvo.
func (s *Store) Load(ctx context.Context) (Preferences, error) {
	b, err := s.kv.Get(ctx, store.KeyPreferences)
	if errors.Is(err, store.ErrNotFound) {
		return Preferences{}.Normalized(), nil
	}
	if err != nil {
		return Preferences{}, err
	}
	var p Preferences
	if err := json.Unmarshal(b, &p); err != nil {
		return Preferences{}, fmt.Errorf("preferências corrompidas: %w", err)
	}
	return p.Normalized(), nil
}

func (s *Store) Save(ctx context.Context, p Preferences) (Preferences, error) {
	p = p.Normalized()
	if err := p.Validate(); err != nil {
		return Preferences{}, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return Preferences{}, err
	}
	return p, s.kv.Put(ctx, store.KeyPreferences, b)
}

// Normalized põe siglas em maiúsculas, ordena e remove duplicados, para que
// a mesma escolha sempre gere o mesmo JSON. Listas e mapas nunca ficam nil:
// o app recebe [] e {}, e não null.
func (p Preferences) Normalized() Preferences {
	out := Preferences{
		HiddenCourses: uniqueSorted(p.HiddenCourses, strings.ToUpper),
		HiddenTypes:   uniqueSorted(p.HiddenTypes, func(t domain.DeadlineType) domain.DeadlineType { return t }),
		CourseColors:  map[string]string{},
		Reminders:     map[domain.DeadlineType][]int{},
	}
	for code, color := range p.CourseColors {
		out.CourseColors[strings.ToUpper(strings.TrimSpace(code))] = color
	}
	for t, mins := range p.Reminders {
		m := slices.Clone(mins)
		slices.Sort(m)
		slices.Reverse(m)
		out.Reminders[t] = slices.Compact(m)
	}
	return out
}

func (p Preferences) Validate() error {
	var errs []error
	for _, t := range p.HiddenTypes {
		if _, err := domain.ParseDeadlineType(string(t)); err != nil {
			errs = append(errs, err)
		}
	}
	for code, color := range p.CourseColors {
		if _, ok := calendar.Palette[color]; !ok || code == "" {
			errs = append(errs, fmt.Errorf("cor %q para %q fora da paleta (1–11)", color, code))
		}
	}
	for t, mins := range p.Reminders {
		if _, err := domain.ParseDeadlineType(string(t)); err != nil {
			errs = append(errs, err)
		}
		if len(mins) > MaxReminders {
			errs = append(errs, fmt.Errorf("%s: no máximo %d lembretes", t, MaxReminders))
		}
		for _, m := range mins {
			if m <= 0 || m > MaxReminderMinutes {
				errs = append(errs, fmt.Errorf("%s: lembrete de %d min fora de 1–%d", t, m, MaxReminderMinutes))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

// Hides indica se o usuário escondeu este prazo no app.
func (p Preferences) Hides(d domain.Deadline) bool {
	return slices.Contains(p.HiddenCourses, strings.ToUpper(d.CourseCode)) || slices.Contains(p.HiddenTypes, d.Type)
}

func (p Preferences) Style() calendar.Style {
	s := calendar.Style{Colors: p.CourseColors, Reminders: map[domain.DeadlineType][]time.Duration{}}
	for t, mins := range p.Reminders {
		d := make([]time.Duration, len(mins))
		for i, m := range mins {
			d[i] = time.Duration(m) * time.Minute
		}
		s.Reminders[t] = d
	}
	return s
}

func uniqueSorted[T ~string](in []T, norm func(T) T) []T {
	set := collections.Set[T]{}
	for _, v := range in {
		if v = T(strings.TrimSpace(string(norm(v)))); v != "" {
			set.Add(v)
		}
	}
	out := make([]T, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
