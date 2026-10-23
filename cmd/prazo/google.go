package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"prazo/internal/calendar/google"
	"prazo/internal/crypto"
	"prazo/internal/store/filekv"
)

var (
	errNoMasterKey   = errors.New("defina PRAZO_MASTER_KEY (gere uma com \"prazo keygen\")")
	errNoCredentials = errors.New("defina PRAZO_GOOGLE_CREDENTIALS com o caminho do JSON da credencial OAuth")
)

// googleHTTP é o cliente base das chamadas ao Google, inclusive a renovação
// do token; o Sink cuida dos retries.
var googleHTTP = &http.Client{Timeout: 30 * time.Second}

func (a *app) secrets() (*filekv.Store, error) {
	if a.cfg.Storage.MasterKey == nil {
		return nil, errNoMasterKey
	}
	sealer, err := crypto.NewSealer(a.cfg.Storage.MasterKey)
	if err != nil {
		return nil, err
	}
	return filekv.New(a.cfg.Storage.DataDir, sealer)
}

func (a *app) oauthConfig() (*oauth2.Config, error) {
	if a.cfg.Google.CredentialsFile == "" {
		return nil, errNoCredentials
	}
	return google.LoadConfig(a.cfg.Google.CredentialsFile)
}

func (a *app) googleSink(ctx context.Context, kv *filekv.Store) (*google.Sink, error) {
	cfg, err := a.oauthConfig()
	if err != nil {
		return nil, err
	}
	tok, err := google.LoadToken(ctx, kv)
	if err != nil {
		return nil, err
	}
	return google.NewSink(ctx, google.HTTPClient(ctx, cfg, tok, googleHTTP))
}
