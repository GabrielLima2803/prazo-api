package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

const maxBody = 64 << 10

// Limites de login: um teto global (protege o Moodle e o servidor) e um por
// conta (impede tentar muitas senhas de uma mesma pessoa por aqui).
const (
	loginGlobalMax     = 20
	loginGlobalWindow  = time.Minute
	loginAccountMax    = 5
	loginAccountWindow = 15 * time.Minute
)

// hstsMaxAge é um ano; os navegadores ignoram o cabeçalho fora do HTTPS.
const hstsMaxAge = "max-age=31536000"

// Política padrão: as respostas da API são JSON e nunca devem rodar como página.
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

type server struct {
	Deps
	logins        *limiter
	accountLogins *limiter
}

// New devolve o handler com todas as rotas. /healthz, o login e o callback do
// Google são públicos; /api/v1 exige uma sessão do app ou o PRAZO_API_TOKEN.
func New(d Deps) http.Handler {
	if d.PlanMaxAge <= 0 {
		d.PlanMaxAge = 5 * time.Minute
	}
	if d.SyncTimeout <= 0 {
		d.SyncTimeout = 3 * time.Minute
	}
	s := &server{
		Deps:          d,
		logins:        newLimiter(loginGlobalMax, loginGlobalWindow),
		accountLogins: newLimiter(loginAccountMax, loginAccountWindow),
	}

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/status", s.status)
	api.HandleFunc("GET /api/v1/meta", s.meta)
	api.HandleFunc("GET /api/v1/deadlines", s.deadlines)
	api.HandleFunc("GET /api/v1/courses", s.courses)
	api.HandleFunc("GET /api/v1/preferences", s.getPreferences)
	api.HandleFunc("PUT /api/v1/preferences", s.putPreferences)
	api.HandleFunc("POST /api/v1/sync", s.sync)
	api.HandleFunc("GET /api/v1/moodle", s.getMoodle)
	api.HandleFunc("POST /api/v1/auth/logout", s.logout)
	api.HandleFunc("POST /api/v1/google/auth", s.googleAuth)
	api.HandleFunc("DELETE /api/v1/google", s.deleteGoogle)
	api.HandleFunc("GET /api/v1/account/export", s.exportAccount)
	api.HandleFunc("DELETE /api/v1/account", s.deleteAccount)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.Handle("/api/", s.authenticate(api))
	mux.HandleFunc("POST /auth/login", s.login)
	mux.HandleFunc("POST /auth/ical", s.loginICal)
	// Quem chega aqui é o navegador vindo do Google; o state prova que a
	// autorização começou pelo app autenticado.
	mux.HandleFunc("GET /oauth/google/callback", s.googleCallback)
	mux.HandleFunc("GET /privacidade", s.privacyPage)
	return s.logRequests(secureHeaders(cors(d.CORSOrigins, mux)))
}

type ctxKey struct{}

// tenantOf devolve o usuário da requisição, já autenticada.
func tenantOf(r *http.Request) *tenant.Tenant { return r.Context().Value(ctxKey{}).(*tenant.Tenant) }

// authenticate resolve o usuário pela sessão do app (ou o PRAZO_API_TOKEN,
// que age como AdminTenant) e o põe no contexto da requisição.
func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		t, err := s.resolve(r.Context(), token, bearer)
		if err != nil {
			writeCodedError(w, http.StatusUnauthorized, "unauthorized", errors.New("sessão expirada; entre de novo"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, t)))
	})
}

func (s *server) resolve(ctx context.Context, token string, bearer bool) (*tenant.Tenant, error) {
	if !bearer || token == "" {
		return nil, errors.New("sem token")
	}
	if s.Token != "" && s.AdminTenant != nil && subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) == 1 {
		return s.AdminTenant(ctx)
	}
	userID, ok := s.Sessions.User(ctx, token)
	if !ok {
		return nil, errors.New("sessão desconhecida")
	}
	return s.Tenants.Get(ctx, userID)
}

// limiter é uma janela fixa por chave. As janelas vencidas são descartadas a
// cada chamada; o limite global vem antes, então o mapa fica pequeno.
type limiter struct {
	mu     gosync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newLimiter(max int, d time.Duration) *limiter {
	return &limiter{max: max, window: d, now: time.Now, hits: map[string]*window{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, w := range l.hits {
		if now.Sub(w.start) > l.window {
			delete(l.hits, k)
		}
	}
	w := l.hits[key]
	if w == nil {
		w = &window{start: now}
		l.hits[key] = w
	}
	w.count++
	return w.count <= l.max
}

// cors libera só as origens configuradas (PRAZO_CORS_ORIGINS), para o app
// rodando no navegador em desenvolvimento. O app nativo não precisa de CORS.
// "*" libera qualquer origem: a proteção real é o token, que nunca vai em
// cookie, mas em produção é melhor deixar vazio.
func cors(origins []string, next http.Handler) http.Handler {
	if len(origins) == 0 {
		return next
	}
	anyOrigin := slices.Contains(origins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (anyOrigin || slices.Contains(origins, origin)) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders vale para todas as respostas. As páginas HTML (callback do
// Google e privacidade) trocam a CSP pela delas.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Strict-Transport-Security", hstsMaxAge)
		h.Set("Content-Security-Policy", apiCSP)
		// As respostas trazem dados pessoais: nada de cache em proxy ou disco.
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/healthz") {
			return
		}
		s.Log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", rec.code,
			"duracao", time.Since(start).Round(time.Millisecond))
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, errorBody{Error: err.Error()})
}

// internalError registra o erro completo e responde uma mensagem genérica:
// detalhes internos (caminhos, chaves, respostas de terceiros) não saem daqui.
func (s *server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("erro interno", "method", r.Method, "path", r.URL.Path, "err", err)
	writeCodedError(w, http.StatusInternalServerError, "internal", errInternal)
}

var errInternal = errors.New("erro interno no servidor do Prazo; tente de novo em instantes")

func writeCodedError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, errorBody{Error: err.Error(), Code: code})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("JSON inválido: %w", err))
		return false
	}
	return true
}

// syncInBackground sincroniza fora da requisição: se o app fechar, a
// sincronização termina mesmo assim.
func (s *server) syncInBackground(t *tenant.Tenant) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.SyncTimeout)
		defer cancel()
		t.Runner.RunNow(ctx) // ErrBusy e falhas já são logados pelo Runner
	}()
}
