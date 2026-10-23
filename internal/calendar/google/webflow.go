package google

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

const (
	CallbackPath = "/oauth/google/callback"
	pendingTTL   = 10 * time.Minute
)

var (
	ErrWebNotConfigured = errors.New("autorização pelo app não configurada no servidor (PRAZO_PUBLIC_URL e PRAZO_GOOGLE_WEB_CREDENTIALS)")
	ErrUnknownState     = errors.New("autorização expirada ou desconhecida; comece de novo pelo app")
	ErrInvalidReturnTo  = errors.New("endereço de retorno inválido")
)

// Só esquemas de app (prazo://, exp://) servem de retorno: http(s) faria do
// callback um redirecionador aberto.
var appScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

// AppAuth é o fluxo OAuth de servidor web, usado quando a autorização começa
// no celular: o Google devolve o código para PRAZO_PUBLIC_URL (HTTPS), e não
// para 127.0.0.1.
type AppAuth struct {
	cfg *oauth2.Config
	kv  store.KV
	now func() time.Time

	mu      gosync.Mutex
	pending map[string]pendingAuth
}

type pendingAuth struct {
	verifier string
	returnTo string
	expires  time.Time
}

// NewAppAuth recebe a credencial "Aplicativo da Web" e a URL pública do
// servidor; o redirect registrado no Google Cloud é publicURL+CallbackPath.
func NewAppAuth(cfg *oauth2.Config, publicURL string, kv store.KV) *AppAuth {
	c := *cfg
	c.RedirectURL = strings.TrimRight(publicURL, "/") + CallbackPath
	return &AppAuth{cfg: &c, kv: kv, now: time.Now, pending: map[string]pendingAuth{}}
}

func (a *AppAuth) RedirectURL() string { return a.cfg.RedirectURL }

// Start devolve a URL de consentimento. returnTo (opcional) é para onde o
// navegador volta ao final, normalmente o deep link do app.
func (a *AppAuth) Start(returnTo string) (string, error) {
	if returnTo != "" && !validReturnTo(returnTo) {
		return "", ErrInvalidReturnTo
	}
	state := rand.Text()
	verifier := oauth2.GenerateVerifier()

	a.mu.Lock()
	now := a.now()
	for k, p := range a.pending {
		if now.After(p.expires) {
			delete(a.pending, k)
		}
	}
	a.pending[state] = pendingAuth{verifier: verifier, returnTo: returnTo, expires: now.Add(pendingTTL)}
	a.mu.Unlock()

	return a.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier)), nil
}

// Finish troca o código pelo token e o guarda. Cada state vale uma vez.
func (a *AppAuth) Finish(ctx context.Context, state, code string) (string, error) {
	a.mu.Lock()
	p, ok := a.pending[state]
	delete(a.pending, state)
	a.mu.Unlock()
	if !ok || a.now().After(p.expires) {
		return "", ErrUnknownState
	}

	tok, err := a.cfg.Exchange(ctx, code, oauth2.VerifierOption(p.verifier))
	if err != nil {
		return p.returnTo, fmt.Errorf("trocar código por token: %w", err)
	}
	if tok.RefreshToken == "" {
		return p.returnTo, ErrNoRefreshToken
	}
	if granted, ok := tok.Extra("scope").(string); ok && !slices.Contains(strings.Fields(granted), Scope) {
		return p.returnTo, ErrScopeDenied
	}
	return p.returnTo, SaveToken(ctx, a.kv, ClientWeb, tok)
}

func (a *AppAuth) Disconnect(ctx context.Context) error {
	return a.kv.Delete(ctx, store.KeyGoogleToken)
}

func validReturnTo(s string) bool {
	u, err := url.Parse(s)
	return err == nil && appScheme.MatchString(s) && u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "javascript"
}
