package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
	"github.com/GabrielLima2803/prazo-sync/internal/source/webservice"
)

func (s *server) getMoodle(w http.ResponseWriter, r *http.Request) {
	m, err := s.Accounts.Moodle(r.Context())
	switch {
	case errors.Is(err, account.ErrNotConnected):
		writeJSON(w, http.StatusOK, MoodleStatus{})
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, toMoodleStatus(m, 0))
	}
}

// moodleLogin recebe a senha só para trocar por um token; ela não é logada
// nem guardada.
func (s *server) moodleLogin(w http.ResponseWriter, r *http.Request) {
	var req moodleLogin
	if !decode(w, r, &req) {
		return
	}
	m, events, err := s.Accounts.ConnectWithLogin(r.Context(), req.BaseURL, req.Username, req.Password)
	s.connected(w, m, events, err)
}

func (s *server) moodleICal(w http.ResponseWriter, r *http.Request) {
	var req moodleICal
	if !decode(w, r, &req) {
		return
	}
	m, events, err := s.Accounts.ConnectWithICal(r.Context(), req.ICalURL)
	s.connected(w, m, events, err)
}

func (s *server) deleteMoodle(w http.ResponseWriter, r *http.Request) {
	if err := s.Accounts.Disconnect(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.Runner.Invalidate()
	writeJSON(w, http.StatusOK, MoodleStatus{})
}

// connected responde e, se deu certo, já sincroniza em segundo plano para que
// a agenda apareça sem o usuário precisar apertar nada.
func (s *server) connected(w http.ResponseWriter, m account.Moodle, events int, err error) {
	if err != nil {
		status, code := connectError(err)
		writeCodedError(w, status, code, err)
		return
	}
	s.Runner.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.SyncTimeout)
		defer cancel()
		s.Runner.RunNow(ctx) // ErrBusy e falhas já são logados pelo Runner
	}()
	writeJSON(w, http.StatusOK, toMoodleStatus(m, events))
}

func connectError(err error) (int, string) {
	switch {
	case errors.Is(err, account.ErrNotOwner):
		return http.StatusForbidden, "not_owner"
	case errors.Is(err, account.ErrMissingUserID):
		return http.StatusUnprocessableEntity, "invalid_url"
	case errors.Is(err, webservice.ErrInvalidLogin):
		return http.StatusUnprocessableEntity, "invalid_login"
	case errors.Is(err, webservice.ErrServiceUnavailable):
		return http.StatusUnprocessableEntity, "service_unavailable"
	case errors.Is(err, account.ErrExportUnavailable):
		return http.StatusUnprocessableEntity, "export_unavailable"
	case errors.Is(err, account.ErrInvalidBaseURL), errors.Is(err, ical.ErrInvalidURL):
		return http.StatusUnprocessableEntity, "invalid_url"
	case errors.Is(err, ical.ErrNotICalendar):
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

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow() {
		writeCodedError(w, http.StatusTooManyRequests, "too_many_attempts", errors.New("muitas tentativas; espere um minuto"))
		return
	}
	var req moodleLogin
	if !decode(w, r, &req) {
		return
	}
	m, events, err := s.Accounts.ConnectWithLogin(r.Context(), req.BaseURL, req.Username, req.Password)
	s.loggedIn(w, r, m, events, err)
}

func (s *server) loginICal(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow() {
		writeCodedError(w, http.StatusTooManyRequests, "too_many_attempts", errors.New("muitas tentativas; espere um minuto"))
		return
	}
	var req moodleICal
	if !decode(w, r, &req) {
		return
	}
	m, events, err := s.Accounts.ConnectWithICal(r.Context(), req.ICalURL)
	s.loggedIn(w, r, m, events, err)
}

// loggedIn emite a sessão do app depois que a conta do Moodle foi conferida
// e salva, e já dispara a primeira sincronização.
func (s *server) loggedIn(w http.ResponseWriter, r *http.Request, m account.Moodle, events int, err error) {
	if err != nil {
		status, code := connectError(err)
		if code == "not_owner" {
			s.Log.Warn("login recusado: outra conta do Moodle")
		}
		writeCodedError(w, status, code, err)
		return
	}
	token, err := s.Sessions.Issue(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.Runner.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.SyncTimeout)
		defer cancel()
		s.Runner.RunNow(ctx)
	}()
	writeJSON(w, http.StatusOK, loginResponse{Token: token, Moodle: toMoodleStatus(m, events)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.Sessions != nil {
		if err := s.Sessions.Revoke(r.Context(), token); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
