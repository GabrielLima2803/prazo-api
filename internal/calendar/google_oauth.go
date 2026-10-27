package calendar

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"

	"github.com/GabrielLima2803/prazo-sync/internal/storage"
)

var (
	ErrNotAuthorized  = errors.New("o Google ainda não foi autorizado; rode \"prazo auth\"")
	ErrNoRefreshToken = errors.New("o Google não devolveu um refresh token; remova o acesso do Prazo em https://myaccount.google.com/permissions e rode \"prazo auth\" de novo")
	ErrScopeDenied    = errors.New("a permissão de agenda não foi concedida; rode \"prazo auth\" e marque a caixa de acesso às agendas")
	ErrStateMismatch  = errors.New("resposta de autorização com state inválido")
)

func statusCode(err error) int {
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		return ge.Code
	}
	return 0
}

func hasReason(err error, reasons ...string) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		return false
	}
	for _, item := range ge.Errors {
		for _, r := range reasons {
			if item.Reason == r {
				return true
			}
		}
	}
	return false
}

// retryable segue a orientação do Google: 429, 5xx e 403 por cota.
func retryable(err error) bool {
	code := statusCode(err)
	return code == http.StatusTooManyRequests || code >= 500 ||
		(code == http.StatusForbidden && hasReason(err, "rateLimitExceeded", "userRateLimitExceeded"))
}

// classify converte erros de autenticação em calendar.ErrAuth, para que o
// sync pare em vez de falhar evento por evento.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var re *oauth2.RetrieveError
	if statusCode(err) == http.StatusUnauthorized || (errors.As(err, &re) && re.ErrorCode == "invalid_grant") {
		return errors.Join(ErrAuth, err)
	}
	return err
}

// LoadOAuthConfig lê o JSON da credencial OAuth "App para computador" baixado do
// Google Cloud Console.
func LoadOAuthConfig(path string) (*oauth2.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("credencial do Google: %w", err)
	}
	cfg, err := googleoauth.ConfigFromJSON(b, GoogleScope)
	if err != nil {
		return nil, fmt.Errorf("credencial do Google em %s: %w", path, err)
	}
	return cfg, nil
}

// LoadWebOAuthConfig lê a credencial "Aplicativo da Web". Não usa
// google.ConfigFromJSON porque ela exige redirect_uris no arquivo, e o
// redirecionamento vem de PRAZO_PUBLIC_URL (o JSON baixado antes de cadastrar
// o URI não traz o campo).
func LoadWebOAuthConfig(path string) (*oauth2.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("credencial web do Google: %w", err)
	}
	var f struct {
		Web *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"web"`
	}
	if err := json.Unmarshal(b, &f); err != nil || f.Web == nil || f.Web.ClientID == "" || f.Web.ClientSecret == "" {
		return nil, fmt.Errorf("credencial web do Google em %s: esperava uma credencial do tipo \"Aplicativo da Web\"", path)
	}
	return &oauth2.Config{
		ClientID:     f.Web.ClientID,
		ClientSecret: f.Web.ClientSecret,
		Endpoint:     googleoauth.Endpoint,
		Scopes:       []string{GoogleScope},
	}, nil
}

// Authorize executa o fluxo de app instalado: sobe um servidor em addr, pede
// que o usuário abra a URL recebida em show e troca o código pelo token.
// O redirect é sempre para 127.0.0.1 na porta escutada, o que permite
// escutar em 0.0.0.0 dentro de um container com a porta publicada.
func Authorize(ctx context.Context, cfg *oauth2.Config, addr string, show func(authURL string)) (*oauth2.Token, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("escutar em %s: %w", addr, err)
	}
	c := *cfg
	c.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	state := rand.Text()
	verifier := oauth2.GenerateVerifier()
	codes := make(chan callback, 1)
	srv := &http.Server{
		Handler:           callbackHandler(state, codes),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go srv.Serve(ln)
	defer srv.Close()

	show(c.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier)))

	var cb callback
	select {
	case cb = <-codes:
	case <-ctx.Done():
		return nil, fmt.Errorf("aguardando a autorização no navegador: %w", ctx.Err())
	}
	if cb.err != nil {
		return nil, cb.err
	}

	tok, err := c.Exchange(ctx, cb.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("trocar código por token: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	if granted, ok := tok.Extra("scope").(string); ok && !slices.Contains(strings.Fields(granted), GoogleScope) {
		return nil, ErrScopeDenied
	}
	return tok, nil
}

type callback struct {
	code string
	err  error
}

func callbackHandler(state string, codes chan<- callback) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/" || (q.Get("code") == "" && q.Get("error") == "") {
			http.NotFound(w, r) // favicon e afins
			return
		}
		var cb callback
		switch {
		case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1:
			cb.err = ErrStateMismatch
		case q.Get("error") != "":
			cb.err = fmt.Errorf("autorização recusada: %s", q.Get("error"))
		default:
			cb.code = q.Get("code")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		msg := "Prazo autorizado. Pode fechar esta aba e voltar ao terminal."
		if cb.err != nil {
			w.WriteHeader(http.StatusBadRequest)
			msg = "Falhou: " + cb.err.Error()
		}
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Prazo</title><p style=\"font:16px system-ui;margin:3em\">%s</p>", html.EscapeString(msg))

		select {
		case codes <- cb:
		default: // já recebemos uma resposta; ignora recargas da página
		}
	})
}

// OAuthClient identifica qual credencial OAuth emitiu o token: o refresh só
// funciona com a mesma credencial.
type OAuthClient string

const (
	ClientInstalled OAuthClient = "installed" // "App para computador": prazo auth no terminal
	ClientWeb       OAuthClient = "web"       // "Aplicativo da Web": autorização pelo app
)

// StoredToken é o que fica cifrado no KV.
type StoredToken struct {
	Client OAuthClient   `json:"client"`
	Token  *oauth2.Token `json:"token"`
}

func SaveToken(ctx context.Context, kv storage.KV, client OAuthClient, tok *oauth2.Token) error {
	b, err := json.Marshal(StoredToken{Client: client, Token: tok})
	if err != nil {
		return err
	}
	return kv.Put(ctx, storage.KeyGoogleToken, b)
}

// LoadToken aceita também o formato antigo (oauth2.Token puro), gravado pelo
// "prazo auth" antes de existir a autorização pelo app.
func LoadToken(ctx context.Context, kv storage.KV) (StoredToken, error) {
	b, err := kv.Get(ctx, storage.KeyGoogleToken)
	if errors.Is(err, storage.ErrNotFound) {
		return StoredToken{}, ErrNotAuthorized
	}
	if err != nil {
		return StoredToken{}, err
	}
	var st StoredToken
	if err := json.Unmarshal(b, &st); err != nil {
		return StoredToken{}, fmt.Errorf("token do Google corrompido: %w", err)
	}
	if st.Token == nil {
		var legacy oauth2.Token
		if err := json.Unmarshal(b, &legacy); err != nil || legacy.RefreshToken == "" {
			return StoredToken{}, errors.New("token do Google corrompido")
		}
		st = StoredToken{Client: ClientInstalled, Token: &legacy}
	}
	return st, nil
}

// GoogleHTTPClient devolve um cliente que injeta e renova o access token. base
// define timeouts e transporte das chamadas (inclusive a renovação).
func GoogleHTTPClient(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token, base *http.Client) *http.Client {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, base)
	return oauth2.NewClient(ctx, cfg.TokenSource(ctx, tok))
}
