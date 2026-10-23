package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
	"github.com/GabrielLima2803/prazo-sync/internal/source/webservice"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

var errTooManyAttempts = errors.New("muitas tentativas; espere um minuto")

// login recebe a senha só para trocar por um token do Moodle; ela não é
// logada nem guardada.
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow() {
		writeCodedError(w, http.StatusTooManyRequests, "too_many_attempts", errTooManyAttempts)
		return
	}
	var req moodleLogin
	if !decode(w, r, &req) {
		return
	}
	m, err := s.Auth.Login(r.Context(), req.BaseURL, req.Username, req.Password)
	s.enter(w, r, m, err)
}

func (s *server) loginICal(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow() {
		writeCodedError(w, http.StatusTooManyRequests, "too_many_attempts", errTooManyAttempts)
		return
	}
	var req moodleICal
	if !decode(w, r, &req) {
		return
	}
	m, err := s.Auth.FromICal(req.ICalURL)
	s.enter(w, r, m, err)
}

// enter cria (ou reencontra) o usuário pela identidade no Moodle, guarda a
// conta no espaço dele, emite a sessão do app e já dispara a primeira
// sincronização.
func (s *server) enter(w http.ResponseWriter, r *http.Request, m account.Moodle, err error) {
	if err != nil {
		status, code := connectError(err)
		writeCodedError(w, status, code, err)
		return
	}
	u := tenant.User{ID: tenant.IDFor(m.Host(), m.UserID), Host: m.Host(), MoodleUserID: m.UserID, Name: m.FullName}
	t, err := s.Tenants.Register(r.Context(), u)
	if err != nil {
		status, code := connectError(err)
		writeCodedError(w, status, code, err)
		return
	}
	events, err := t.Accounts.Save(r.Context(), m)
	if err != nil {
		status, code := connectError(err)
		writeCodedError(w, status, code, err)
		return
	}
	token, err := s.Sessions.Issue(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	t.Runner.Invalidate()
	s.syncInBackground(t)
	saved, _ := t.Accounts.Moodle(r.Context())
	writeJSON(w, http.StatusOK, loginResponse{Token: token, Moodle: toMoodleStatus(saved, events)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := s.Sessions.Revoke(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) getMoodle(w http.ResponseWriter, r *http.Request) {
	m, err := tenantOf(r).Accounts.Moodle(r.Context())
	switch {
	case errors.Is(err, account.ErrNotConnected):
		writeJSON(w, http.StatusOK, MoodleStatus{})
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, toMoodleStatus(m, 0))
	}
}

func connectError(err error) (int, string) {
	switch {
	case errors.Is(err, account.ErrHostNotAllowed):
		return http.StatusForbidden, "host_not_allowed"
	case errors.Is(err, tenant.ErrTooManyUsers):
		return http.StatusForbidden, "too_many_users"
	case errors.Is(err, webservice.ErrInvalidLogin):
		return http.StatusUnprocessableEntity, "invalid_login"
	case errors.Is(err, webservice.ErrServiceUnavailable):
		return http.StatusUnprocessableEntity, "service_unavailable"
	case errors.Is(err, account.ErrExportUnavailable):
		return http.StatusUnprocessableEntity, "export_unavailable"
	case errors.Is(err, account.ErrInvalidBaseURL), errors.Is(err, ical.ErrInvalidURL), errors.Is(err, account.ErrMissingUserID):
		return http.StatusUnprocessableEntity, "invalid_url"
	case errors.Is(err, ical.ErrAuthRejected), errors.Is(err, ical.ErrNotICalendar):
		return http.StatusUnprocessableEntity, "not_icalendar"
	case errors.Is(err, account.ErrEmptyFeed):
		return http.StatusUnprocessableEntity, "empty_feed"
	}
	return http.StatusBadGateway, "moodle_unreachable"
}

func toMoodleStatus(m account.Moodle, events int) MoodleStatus {
	st := MoodleStatus{Connected: true, Method: m.Method, BaseURL: m.BaseURL, FullName: m.FullName, SiteName: m.SiteName, Events: events, NeedsReauth: m.NeedsReauth}
	if !m.ConnectedAt.IsZero() {
		at := m.ConnectedAt
		st.ConnectedAt = &at
	}
	return st
}
