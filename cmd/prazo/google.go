package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/google"
	"github.com/GabrielLima2803/prazo-sync/internal/crypto"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/store/filekv"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
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

// target monta o destino a cada execução: lê o token e as preferências na
// hora, para que "prazo auth" e mudanças no app valham sem reiniciar.
func (a *app) target(kv *filekv.Store) sync.TargetFunc {
	ps := prefs.NewStore(kv)
	return func(ctx context.Context) (sync.Target, error) {
		sink, err := a.googleSink(ctx, kv)
		if err != nil {
			return sync.Target{}, err
		}
		p, err := ps.Load(ctx)
		if err != nil {
			return sync.Target{}, err
		}
		return sync.Target{Sink: sink, State: kv, Calendar: a.calendarSpec(), Prefs: p}, nil
	}
}

func (a *app) calendarSpec() calendar.Spec {
	return calendar.Spec{
		Summary:     a.cfg.Google.CalendarName,
		Description: "Prazos do Moodle, mantidos automaticamente pelo Prazo. Alterações manuais podem ser sobrescritas.",
		TimeZone:    a.cfg.Location.String(),
	}
}

// webOAuthConfig é a credencial "Aplicativo da Web"; nil, nil quando a
// autorização pelo app não foi configurada.
func (a *app) webOAuthConfig() (*oauth2.Config, error) {
	if a.cfg.Google.WebCredentialsFile == "" {
		return nil, nil
	}
	return google.LoadConfig(a.cfg.Google.WebCredentialsFile)
}

// googleSink renova o token com a mesma credencial que o emitiu.
func (a *app) googleSink(ctx context.Context, kv *filekv.Store) (*google.Sink, error) {
	st, err := google.LoadToken(ctx, kv)
	if err != nil {
		return nil, err
	}
	var cfg *oauth2.Config
	if st.Client == google.ClientWeb {
		if cfg, err = a.webOAuthConfig(); err == nil && cfg == nil {
			err = errors.New("o token veio da autorização pelo app: defina PRAZO_GOOGLE_WEB_CREDENTIALS")
		}
	} else {
		cfg, err = a.oauthConfig()
	}
	if err != nil {
		return nil, err
	}
	return google.NewSink(ctx, google.HTTPClient(ctx, cfg, st.Token, googleHTTP))
}
