package api

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar/google"
)

var errGoogleAppAuthOff = errors.New("o servidor não está configurado para autorizar o Google pelo app")

func (s *server) googleAuth(w http.ResponseWriter, r *http.Request) {
	if s.GoogleAuth == nil {
		writeCodedError(w, http.StatusNotImplemented, "google_app_auth_unavailable", errGoogleAppAuthOff)
		return
	}
	var req googleAuthRequest
	if !decode(w, r, &req) {
		return
	}
	authURL, err := s.GoogleAuth.Start(tenantOf(r).User.ID, req.ReturnTo)
	if err != nil {
		writeCodedError(w, http.StatusUnprocessableEntity, "invalid_return_to", err)
		return
	}
	writeJSON(w, http.StatusOK, googleAuthResponse{AuthURL: authURL})
}

func (s *server) deleteGoogle(w http.ResponseWriter, r *http.Request) {
	if s.GoogleAuth == nil {
		writeCodedError(w, http.StatusNotImplemented, "google_app_auth_unavailable", errGoogleAppAuthOff)
		return
	}
	err := s.GoogleAuth.Disconnect(r.Context(), tenantOf(r).User.ID)
	switch {
	case errors.Is(err, google.ErrRevoke):
		// O token já saiu daqui; só o Google não confirmou.
		s.Log.Warn("revogar acesso do Google", "user", tenantOf(r).User.ID, "err", err)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "revoked": false})
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "revoked": true})
	}
}

func (s *server) googleCallback(w http.ResponseWriter, r *http.Request) {
	if s.GoogleAuth == nil {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	page := callbackPage{OK: true}
	switch {
	case q.Get("error") != "":
		page = callbackPage{Message: "A autorização foi cancelada no Google."}
	default:
		userID, returnTo, err := s.GoogleAuth.Finish(r.Context(), q.Get("state"), q.Get("code"))
		page.ReturnTo = template.URL(returnTo) // validado no Start: só esquemas de app
		if err != nil {
			s.Log.Warn("autorização do Google falhou", "user", userID, "err", err)
			page.OK, page.Message = false, callbackMessage(err)
		} else {
			s.Log.Info("Google autorizado pelo app", "user", userID)
			if t, err := s.Tenants.Get(r.Context(), userID); err == nil {
				s.syncInBackground(t)
			}
		}
	}
	if page.ReturnTo != "" {
		result := "ok"
		if !page.OK {
			result = "error"
		}
		page.ReturnTo = template.URL(fmt.Sprintf("%s?google=%s", page.ReturnTo, url.QueryEscape(result)))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	if !page.OK {
		w.WriteHeader(http.StatusBadRequest)
	}
	callbackTmpl.Execute(w, page)
}

// pageCSP vale para as páginas HTML do servidor: só estilo e script embutidos.
const pageCSP = "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// callbackMessage traduz o erro para quem está no navegador, sem repetir
// detalhes da resposta do Google.
func callbackMessage(err error) string {
	switch {
	case errors.Is(err, google.ErrUnknownState):
		return "A autorização expirou. Comece de novo pelo app."
	case errors.Is(err, google.ErrScopeDenied):
		return "Marque a permissão de acesso às agendas e tente de novo pelo app."
	case errors.Is(err, google.ErrNoRefreshToken):
		return "O Google não liberou o acesso contínuo. Remova o Prazo em myaccount.google.com/permissions e tente de novo pelo app."
	}
	return "Não foi possível concluir a autorização. Tente de novo pelo app."
}

type callbackPage struct {
	OK       bool
	Message  string
	ReturnTo template.URL
}

var callbackTmpl = template.Must(template.New("cb").Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Prazo</title>
<style>
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0B0D12;color:#F2F4F8;font:16px/1.5 system-ui,sans-serif}
main{max-width:340px;padding:32px;text-align:center}
.icon{width:72px;height:72px;border-radius:36px;display:grid;place-items:center;margin:0 auto 20px;font-size:36px}
.ok{background:rgba(52,211,153,.15);color:#34D399}.err{background:rgba(248,113,113,.15);color:#F87171}
h1{font-size:22px;margin:0 0 8px}p{color:#8B93A7;margin:0 0 24px}
a{display:block;padding:14px;border-radius:12px;background:linear-gradient(135deg,#6D4BFF,#B05CFF);color:#fff;text-decoration:none;font-weight:600}
</style></head><body><main>
{{if .OK}}<div class="icon ok">✓</div><h1>Google conectado!</h1><p>Seus prazos já estão indo para a agenda. Pode voltar ao app.</p>
{{else}}<div class="icon err">!</div><h1>Não deu certo</h1><p>{{.Message}}</p>{{end}}
{{if .ReturnTo}}<a href="{{.ReturnTo}}">Voltar ao Prazo</a><script>location.href={{.ReturnTo}}</script>{{end}}
</main></body></html>`))
