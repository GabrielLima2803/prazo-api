package api

import (
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	errTooManyAttempts = errors.New("muitas tentativas; espere um minuto")
	errMoodleDown      = errors.New("não foi possível falar com o Moodle agora; tente de novo em instantes")
)

// allowLogin aplica o limite global e o da conta (Moodle + usuário).
func (s *server) allowLogin(w http.ResponseWriter, account string) bool {
	if s.logins.allow("") && s.accountLogins.allow(strings.ToLower(account)) {
		return true
	}
	writeCodedError(w, http.StatusTooManyRequests, "too_many_attempts", errTooManyAttempts)
	return false
}

// login recebe a senha só para trocar por um token do Moodle; ela não é
// logada nem guardada.
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req MoodleLogin
	if !decode(w, r, &req) {
		return
	}
	if !s.allowLogin(w, req.BaseURL+"|"+strings.TrimSpace(req.Username)) {
		return
	}
	m, err := s.Auth.Login(r.Context(), req.BaseURL, req.Username, req.Password)
	s.enter(w, r, m, err)
}

func (s *server) loginICal(w http.ResponseWriter, r *http.Request) {
	var req MoodleICal
	if !decode(w, r, &req) {
		return
	}
	host, userID := moodle.IdentityFromICal(req.ICalURL)
	if !s.allowLogin(w, fmt.Sprintf("ical|%s|%d", host, userID)) {
		return
	}
	m, err := s.Auth.FromICal(req.ICalURL)
	s.enter(w, r, m, err)
}

// enter cria (ou reencontra) o usuário pela identidade no Moodle, guarda a
// conta no espaço dele, emite a sessão do app e já dispara a primeira
// sincronização.
func (s *server) enter(w http.ResponseWriter, r *http.Request, m moodle.Account, err error) {
	if err != nil {
		s.connectFailed(w, r, err)
		return
	}
	u := tenant.User{ID: tenant.IDFor(m.Host(), m.UserID), Host: m.Host(), MoodleUserID: m.UserID, Name: m.FullName}
	t, err := s.Tenants.Register(r.Context(), u)
	if err != nil {
		s.connectFailed(w, r, err)
		return
	}
	events, err := t.Accounts.Save(r.Context(), m)
	if err != nil {
		s.connectFailed(w, r, err)
		return
	}
	token, err := s.Sessions.Issue(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	t.Runner.Invalidate()
	s.syncInBackground(t)
	saved, _ := t.Accounts.Moodle(r.Context())
	writeJSON(w, http.StatusOK, LoginResponse{Token: token, Moodle: toMoodleStatus(saved, events)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := s.Sessions.Revoke(r.Context(), token); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) getMoodle(w http.ResponseWriter, r *http.Request) {
	m, err := tenantOf(r).Accounts.Moodle(r.Context())
	switch {
	case errors.Is(err, moodle.ErrNotConnected):
		writeJSON(w, http.StatusOK, MoodleStatus{})
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toMoodleStatus(m, 0))
	}
}

// connectFailed responde os erros conhecidos com a mensagem deles; o resto
// (rede, disco) é registrado e respondido de forma genérica.
func (s *server) connectFailed(w http.ResponseWriter, r *http.Request, err error) {
	status, code := connectError(err)
	if code == codeMoodleUnreachable {
		s.Log.Warn("login falhou", "path", r.URL.Path, "err", err)
		err = errMoodleDown
	}
	writeCodedError(w, status, code, err)
}

const codeMoodleUnreachable = "moodle_unreachable"

func connectError(err error) (int, string) {
	switch {
	case errors.Is(err, moodle.ErrHostNotAllowed):
		return http.StatusForbidden, "host_not_allowed"
	case errors.Is(err, tenant.ErrTooManyUsers):
		return http.StatusForbidden, "too_many_users"
	case errors.Is(err, moodle.ErrInvalidLogin):
		return http.StatusUnprocessableEntity, "invalid_login"
	case errors.Is(err, moodle.ErrAuthBackendDown):
		return http.StatusServiceUnavailable, "moodle_auth_down"
	case errors.Is(err, moodle.ErrServiceUnavailable):
		return http.StatusUnprocessableEntity, "service_unavailable"
	case errors.Is(err, moodle.ErrExportUnavailable):
		return http.StatusUnprocessableEntity, "export_unavailable"
	case errors.Is(err, moodle.ErrInvalidBaseURL), errors.Is(err, moodle.ErrInvalidURL), errors.Is(err, moodle.ErrMissingUserID):
		return http.StatusUnprocessableEntity, "invalid_url"
	case errors.Is(err, moodle.ErrAuthRejected), errors.Is(err, moodle.ErrNotICalendar):
		return http.StatusUnprocessableEntity, "not_icalendar"
	case errors.Is(err, moodle.ErrEmptyFeed):
		return http.StatusUnprocessableEntity, "empty_feed"
	}
	return http.StatusBadGateway, codeMoodleUnreachable
}

func toMoodleStatus(m moodle.Account, events int) MoodleStatus {
	st := MoodleStatus{Connected: true, Method: m.Method, BaseURL: m.BaseURL, FullName: m.FullName, SiteName: m.SiteName, Events: events, NeedsReauth: m.NeedsReauth}
	if !m.ConnectedAt.IsZero() {
		at := m.ConnectedAt
		st.ConnectedAt = &at
	}
	return st
}
