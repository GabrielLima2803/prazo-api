package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// runHealthcheck consulta o /healthz do próprio servidor. A imagem distroless
// não tem curl; o HEALTHCHECK do Docker usa este comando.
func runHealthcheck(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("healthcheck", "Verifica se o \"prazo serve\" deste container responde (para o HEALTHCHECK do Docker).")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(a.cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("PRAZO_HTTP_ADDR: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/healthz respondeu %d", resp.StatusCode)
	}
	return nil
}
