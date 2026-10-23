package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/api"
	"github.com/GabrielLima2803/prazo-sync/internal/calendar/google"
	"github.com/GabrielLima2803/prazo-sync/internal/session"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

func runServe(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("serve", "Sobe a API usada pelo app e sincroniza com o Google a cada intervalo.")
	addr := fs.String("addr", a.cfg.Server.Addr, "endereço HTTP (0.0.0.0:8080 para o celular acessar pela rede local)")
	interval := fs.Duration("interval", a.cfg.Server.SyncInterval, "intervalo entre sincronizações")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := a.manager(ctx)
	if err != nil {
		return err
	}
	kv, err := a.secrets()
	if err != nil {
		return err
	}
	deps := api.Deps{
		Tenants:  m,
		Auth:     account.NewAuthenticator(a.http, a.cfg.Moodle.AllowedHosts...),
		Sessions: session.New(kv),
		Token:    a.cfg.Server.APIToken,
		AdminTenant: func(ctx context.Context) (*tenant.Tenant, error) {
			return a.cliTenant(ctx, m)
		},
		GoogleReady: func(ctx context.Context, kv store.KV) bool {
			_, err := google.LoadToken(ctx, kv)
			return err == nil
		},
		CalendarName: a.cfg.Google.CalendarName,
		Log:          a.log,
	}
	appAuth, err := a.appAuth(m)
	if err != nil {
		return err
	}
	if appAuth != nil {
		deps.GoogleAuth = appAuth
		fmt.Fprintf(a.stdout, "Autorização do Google pelo app ativa. URI de redirecionamento: %s\n", appAuth.RedirectURL())
	}
	if users, err := m.Users(ctx); err == nil {
		a.log.Info("usuários cadastrados", "total", len(users), "limite", a.cfg.Server.MaxUsers)
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

	go m.Loop(ctx, *interval)
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
func (a *app) appAuth(m *tenant.Manager) (*google.AppAuth, error) {
	if a.cfg.Server.PublicURL == "" || a.cfg.Google.WebCredentialsFile == "" {
		return nil, nil
	}
	cfg, err := a.webOAuthConfig()
	if err != nil {
		return nil, err
	}
	return google.NewAppAuth(cfg, a.cfg.Server.PublicURL, m.Namespace), nil
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
