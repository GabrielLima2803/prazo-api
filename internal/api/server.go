package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const maxBody = 64 << 10

type server struct {
	Deps
}

// New devolve o handler com todas as rotas. /healthz é público; o resto
// exige "Authorization: Bearer <PRAZO_API_TOKEN>".
func New(d Deps) http.Handler {
	if d.PlanMaxAge <= 0 {
		d.PlanMaxAge = 5 * time.Minute
	}
	if d.SyncTimeout <= 0 {
		d.SyncTimeout = 3 * time.Minute
	}
	s := &server{d}

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/status", s.status)
	api.HandleFunc("GET /api/v1/meta", s.meta)
	api.HandleFunc("GET /api/v1/deadlines", s.deadlines)
	api.HandleFunc("GET /api/v1/courses", s.courses)
	api.HandleFunc("GET /api/v1/preferences", s.getPreferences)
	api.HandleFunc("PUT /api/v1/preferences", s.putPreferences)
	api.HandleFunc("POST /api/v1/sync", s.sync)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.Handle("/api/", s.authenticate(api))
	return s.logRequests(cors(mux))
}

func (s *server) authenticate(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("token inválido"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cors libera o app rodando no navegador (Expo web) em desenvolvimento. A
// proteção real é o token, que nunca vai em cookie.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
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
