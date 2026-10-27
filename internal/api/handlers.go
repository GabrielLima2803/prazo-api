package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/source"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	t := tenantOf(r)
	st := Status{
		Now:    time.Now().UTC(),
		Google: Google{Authorized: s.GoogleReady(r.Context(), t.KV), CalendarName: s.CalendarName, AppAuth: s.GoogleAuth != nil},
	}
	if last, ok := t.Runner.Last(); ok {
		run := toRun(last)
		st.LastRun = &run
	}
	if next := t.Runner.NextRun(); !next.IsZero() {
		st.NextRun = &next
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *server) meta(w http.ResponseWriter, _ *http.Request) {
	m := Meta{}
	for i := 1; i <= len(calendar.Palette); i++ {
		id := fmt.Sprint(i)
		m.Palette = append(m.Palette, Color{ID: id, Hex: calendar.Palette[id]})
	}
	for _, t := range domain.DeadlineTypes {
		m.Types = append(m.Types, Type{ID: t, Label: t.Label(), DefaultReminders: minutes(calendar.DefaultReminders[t])})
	}
	writeJSON(w, http.StatusOK, m)
}

// view junta o plano com as preferências, que o app e o sync compartilham.
type view struct {
	plan  sync.Plan
	prefs prefs.Preferences
	style calendar.Style
}

func (s *server) load(ctx context.Context, t *tenant.Tenant, refresh bool) (view, error) {
	maxAge := s.PlanMaxAge
	if refresh {
		maxAge = 0
	}
	plan, err := t.Runner.Plan(ctx, maxAge)
	if err != nil {
		return view{}, err
	}
	p, err := t.Prefs.Load(ctx)
	if err != nil {
		return view{}, err
	}
	return view{plan: plan, prefs: p, style: p.Style()}, nil
}

// planError diferencia "Moodle não conectado" (o app mostra a tela de
// conexão) de uma falha ao ler o Moodle.
func planError(w http.ResponseWriter, err error) {
	if errors.Is(err, account.ErrNotConnected) {
		writeCodedError(w, http.StatusConflict, "moodle_not_connected", err)
		return
	}
	if errors.Is(err, source.ErrUnauthorized) {
		writeCodedError(w, http.StatusConflict, "moodle_reauth", err)
		return
	}
	writeError(w, http.StatusBadGateway, fmt.Errorf("não foi possível ler o Moodle: %w", err))
}

func (s *server) deadlines(w http.ResponseWriter, r *http.Request) {
	v, err := s.load(r.Context(), tenantOf(r), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		planError(w, err)
		return
	}
	out := DeadlineList{Deadlines: make([]Deadline, 0, len(v.plan.Deadlines)), FetchedAt: v.plan.At}
	for _, d := range v.plan.Deadlines {
		out.Deadlines = append(out.Deadlines, v.toDeadline(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) courses(w http.ResponseWriter, r *http.Request) {
	v, err := s.load(r.Context(), tenantOf(r), false)
	if err != nil {
		planError(w, err)
		return
	}
	byCode := map[string]*Course{}
	for _, d := range v.plan.Deadlines {
		if d.CourseCode == "" {
			continue
		}
		c := byCode[d.CourseCode]
		if c == nil {
			colorID := v.style.ColorFor(d.CourseCode)
			c = &Course{Code: d.CourseCode, Name: d.CourseName, ColorID: colorID, Color: calendar.Palette[colorID],
				Hidden: slices.Contains(v.prefs.HiddenCourses, strings.ToUpper(d.CourseCode))}
			byCode[d.CourseCode] = c
		}
		c.Total++
		if d.EndsAt.After(v.plan.At) {
			c.Pending++
			if c.NextDue == nil || d.EndsAt.Before(*c.NextDue) {
				due := d.EndsAt
				c.NextDue = &due
			}
		}
	}
	out := make([]Course, 0, len(byCode))
	for _, c := range byCode {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Course) int { return strings.Compare(a.Code, b.Code) })
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := tenantOf(r).Prefs.Load(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *server) putPreferences(w http.ResponseWriter, r *http.Request) {
	var p prefs.Preferences
	if !decode(w, r, &p) {
		return
	}
	saved, err := tenantOf(r).Prefs.Save(r.Context(), p)
	switch {
	case errors.Is(err, prefs.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, err)
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, saved)
	}
}

// sync roda a sincronização na hora. O contexto não é o da requisição: se o
// app fechar no meio, a sincronização termina mesmo assim.
func (s *server) sync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), s.SyncTimeout)
	defer cancel()
	run, err := tenantOf(r).Runner.RunNow(ctx)
	switch {
	case errors.Is(err, sync.ErrBusy):
		writeError(w, http.StatusConflict, err)
	default:
		writeJSON(w, http.StatusOK, toRun(run))
	}
}

func (v view) toDeadline(d domain.Deadline) Deadline {
	colorID := v.style.ColorFor(d.CourseCode)
	return Deadline{
		ID: d.SourceID, CourseCode: d.CourseCode, CourseName: d.CourseName,
		Type: d.Type, Label: d.Label(), Phase: d.Phase,
		Title: d.Title, DisplayTitle: d.DisplayTitle(), Description: d.Description, URL: d.URL,
		StartsAt: d.StartsAt, EndsAt: d.EndsAt, Instant: d.IsInstant(), Submitted: d.Submitted,
		ColorID: colorID, Color: calendar.Palette[colorID],
		Reminders: minutes(v.style.RemindersFor(d)),
		Hidden:    v.prefs.Hides(d),
	}
}

func toRun(r sync.Run) Run {
	out := Run{
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, OK: r.Err == nil,
		Total: len(r.Report.Plan.Deadlines), Created: r.Report.Created, Updated: r.Report.Updated,
		Unchanged: r.Report.Unchanged, Deleted: r.Report.Deleted, DeleteBlocked: r.Report.DeleteBlocked,
		Hidden: r.Report.Hidden, Failed: r.Report.Failed,
	}
	if r.Err != nil {
		out.Error = r.Err.Error()
	}
	return out
}

func minutes(ds []time.Duration) []int {
	out := make([]int, len(ds))
	for i, d := range ds {
		out[i] = int(d / time.Minute)
	}
	return out
}
