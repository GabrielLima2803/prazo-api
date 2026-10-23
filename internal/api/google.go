package api

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
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
	authURL, err := s.GoogleAuth.Start(req.ReturnTo)
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
	if err := s.GoogleAuth.Disconnect(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
		returnTo, err := s.GoogleAuth.Finish(r.Context(), q.Get("state"), q.Get("code"))
		page.ReturnTo = template.URL(returnTo) // validado no Start: só esquemas de app
		if err != nil {
			s.Log.Warn("autorização do Google falhou", "err", err)
			page.OK, page.Message = false, err.Error()
		} else {
			s.Log.Info("Google autorizado pelo app")
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), s.SyncTimeout)
				defer cancel()
				s.Runner.RunNow(ctx)
			}()
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
	w.Header().Set("Cache-Control", "no-store")
	if !page.OK {
		w.WriteHeader(http.StatusBadRequest)
	}
	callbackTmpl.Execute(w, page)
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
