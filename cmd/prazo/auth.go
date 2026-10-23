package main

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"

	"github.com/GabrielLima2803/prazo-sync/internal/calendar/google"
	"github.com/GabrielLima2803/prazo-sync/internal/crypto"
)

func runAuth(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("auth", "Autoriza o Prazo a criar e manter a própria agenda no Google.\n"+
		"Abre um servidor local para receber a resposta do Google; o token fica cifrado em PRAZO_DATA_DIR.")
	addr := fs.String("addr", "127.0.0.1:0", "endereço do servidor de retorno (no Docker: 0.0.0.0:8085 com a porta publicada)")
	timeout := fs.Duration("timeout", 5*time.Minute, "tempo máximo para concluir a autorização")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := a.oauthConfig()
	if err != nil {
		return err
	}
	kv, err := a.secrets()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, googleHTTP)

	tok, err := google.Authorize(ctx, cfg, *addr, func(authURL string) {
		fmt.Fprintf(a.stdout, "Abra este endereço no navegador e autorize o acesso:\n\n  %s\n\nAguardando a resposta do Google...\n", authURL)
	})
	if err != nil {
		return err
	}
	if err := google.SaveToken(ctx, kv, google.ClientInstalled, tok); err != nil {
		return fmt.Errorf("guardar token: %w", err)
	}
	fmt.Fprintf(a.stdout, "\nAutorizado. Token guardado (cifrado) em %s.\nPróximo passo: prazo sync\n", a.cfg.Storage.DataDir)
	return nil
}

func runKeygen(_ context.Context, a *app, args []string) error {
	fs := newFlagSet("keygen", "Gera 32 bytes aleatórios em base64, para PRAZO_MASTER_KEY ou PRAZO_API_TOKEN.\n"+
		"Trocar a PRAZO_MASTER_KEY invalida os segredos já guardados: será preciso rodar \"prazo auth\" de novo.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintln(a.stdout, crypto.GenerateKey())
	return nil
}
