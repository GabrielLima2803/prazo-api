package api

import (
	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"context"
	"errors"
	"net/http"
	"time"
)

// deleteTimeout cobre esperar a sincronização em andamento terminar.
const deleteTimeout = 5 * time.Minute

// accountExport é a cópia dos dados do titular (LGPD, art. 18, II e V).
// Senhas nunca são guardadas; tokens e a URL do calendário ficam de fora
// porque são credenciais, não informação sobre a pessoa.
type accountExport struct {
	ExportedAt  time.Time        `json:"exportedAt"`
	User        tenant.User      `json:"user"`
	Moodle      MoodleStatus     `json:"moodle"`
	Google      Google           `json:"google"`
	Preferences sync.Preferences `json:"preferences"`
	Note        string           `json:"note"`
}

const exportNote = "Credenciais (token do Moodle, URL do calendário e token do Google) ficam cifradas no servidor e não são exportadas. " +
	"Os prazos não são guardados: vêm do Moodle a cada consulta."

func (s *server) exportAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t := tenantOf(r)
	out := accountExport{
		ExportedAt: time.Now().UTC(),
		User:       t.User,
		Google:     Google{Authorized: s.GoogleReady(ctx, t.KV), CalendarName: s.CalendarName},
		Note:       exportNote,
	}
	m, err := t.Accounts.Moodle(ctx)
	switch {
	case err == nil:
		out.Moodle = toMoodleStatus(m, 0)
	case !errors.Is(err, moodle.ErrNotConnected):
		s.internalError(w, r, err)
		return
	}
	if out.Preferences, err = t.Prefs.Load(ctx); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="prazo-meus-dados.json"`)
	writeJSON(w, http.StatusOK, out)
}

// deleteAccount apaga tudo o que o servidor guarda do usuário: revoga o
// Google, apaga conta do Moodle, preferências e estado, e encerra todas as
// sessões (LGPD, art. 18, VI). A agenda criada no Google continua na conta
// da pessoa, que decide se a apaga.
func (s *server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	t := tenantOf(r)
	// Termina mesmo se o app fechar no meio.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), deleteTimeout)
	defer cancel()

	if s.ForgetGoogle != nil {
		err := s.ForgetGoogle(ctx, t.KV)
		if errors.Is(err, calendar.ErrRevoke) {
			s.Log.Warn("revogar acesso do Google ao excluir a conta", "user", t.User.ID, "err", err)
		} else if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if err := s.Tenants.Delete(ctx, t.User.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	// Por último: se algo acima falhar, o app ainda pode tentar de novo.
	if err := s.Sessions.RevokeUser(ctx, t.User.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
