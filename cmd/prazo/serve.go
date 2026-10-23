package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/api"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/google"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
)

var errNoAPIToken = errors.New("defina PRAZO_API_TOKEN (gere um com \"prazo keygen\"); o app usa esse token para acessar a API")

func runServe(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("serve", "Sobe a API usada pelo app e sincroniza com o Google a cada intervalo.")
	addr := fs.String("addr", a.cfg.Server.Addr, "endereço HTTP (0.0.0.0:8080 para o celular acessar pela rede local)")
	interval := fs.Duration("interval", a.cfg.Server.SyncInterval, "intervalo entre sincronizações")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if a.cfg.Server.APIToken == "" {
		return errNoAPIToken
	}
	kv, err := a.secrets()
	if err != nil {
		return err
	}
	accounts := account.NewService(kv, a.http, a.log, a.cfg.Moodle.ICalURL)
	runner := sync.NewRunner(a.syncService(accounts), a.target(kv), a.log)
	deps := api.Deps{
		Runner:       runner,
		Accounts:     accounts,
		Prefs:        prefs.NewStore(kv),
		Token:        a.cfg.Server.APIToken,
		CalendarName: a.cfg.Google.CalendarName,
		GoogleReady: func(ctx context.Context) bool {
			_, err := google.LoadToken(ctx, kv)
			return err == nil
		},
		Log: a.log,
	}
	appAuth, err := a.appAuth(kv)
	if err != nil {
		return err
	}
	if appAuth != nil {
		deps.GoogleAuth = appAuth
		fmt.Fprintf(a.stdout, "Autorização do Google pelo app ativa. URI de redirecionamento: %s\n", appAuth.RedirectURL())
	}

	srv := &http.Server{
		Handler:           api.New(deps),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      4 * time.Minute, // POST /sync espera a sincronização terminar
		IdleTimeout:       2 * time.Minute,
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("escutar em %s: %w", *addr, err)
	}
	a.log.Info("API no ar", "addr", ln.Addr().String(), "intervalo", interval.String())
	if u := a.cfg.Server.PublicURL; u != "" {
		fmt.Fprintf(a.stdout, "No app, use o servidor: %s\n", u)
	} else if u := reachableURL(ln.Addr()); u != "" {
		fmt.Fprintf(a.stdout, "No app, use o servidor: %s\n", u)
	}

	go runner.Loop(ctx, *interval)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	a.log.Info("encerrando")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// appAuth devolve nil sem erro quando a autorização pelo app não foi
// configurada: o servidor funciona, só sem o botão "Conectar Google".
func (a *app) appAuth(kv store.KV) (*google.AppAuth, error) {
	if a.cfg.Server.PublicURL == "" || a.cfg.Google.WebCredentialsFile == "" {
		return nil, nil
	}
	cfg, err := a.webOAuthConfig()
	if err != nil {
		return nil, err
	}
	return google.NewAppAuth(cfg, a.cfg.Server.PublicURL, kv), nil
}

// reachableURL devolve o endereço pelo qual o celular alcança o servidor na
// rede local. Escutando em todas as interfaces, usa o IP da rota padrão
// (o "dial" UDP não envia nenhum pacote), ignorando as redes do Docker.
func reachableURL(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return ""
	}
	if !tcp.IP.IsUnspecified() {
		return fmt.Sprintf("http://%s", tcp)
	}
	conn, err := net.Dial("udp", "192.0.2.1:9") // TEST-NET-1: nunca roteado de verdade
	if err != nil {
		return ""
	}
	defer conn.Close()
	return fmt.Sprintf("http://%s:%d", conn.LocalAddr().(*net.UDPAddr).IP, tcp.Port)
}
