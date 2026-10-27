package main

import (
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"

	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // binário independente do tzdata do sistema (imagem distroless)

	"github.com/GabrielLima2803/prazo-sync/internal/config"

	"github.com/GabrielLima2803/prazo-sync/internal/logging"
)

// app reúne as dependências compartilhadas pelos comandos.
type app struct {
	cfg    config.Config
	log    *slog.Logger
	http   *httpx.Client
	stdout io.Writer
}

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, a *app, args []string) error
}

var commands = []command{
	{"doctor", "diagnostica a exportação iCal e os web services do Moodle", runDoctor},
	{"keygen", "gera uma chave aleatória (PRAZO_MASTER_KEY, PRAZO_API_TOKEN)", runKeygen},
	{"auth", "autoriza o acesso ao Google Agenda", runAuth},
	{"sync", "sincroniza os prazos com o Google Agenda (--dry-run só mostra)", runSync},
	{"serve", "API do app e sincronização periódica", runServe},
	{"healthcheck", "verifica se o serve responde (Docker)", runHealthcheck},
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stderr)
		return 2
	}
	cmd, ok := findCommand(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "comando desconhecido %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuração inválida:\n%v\n", err)
		return 2
	}
	a := &app{
		cfg:    cfg,
		log:    logging.New(os.Stderr, cfg.Log.Format, cfg.Log.Level),
		http:   httpx.New(httpx.Options{MaxRetries: 3}),
		stdout: os.Stdout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch err := cmd.run(ctx, a, args[1:]); {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	default:
		a.log.Error("falhou", "cmd", cmd.name, "err", err)
		return 1
	}
}

func findCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "prazo: sincroniza prazos do Moodle com o Google Agenda.\n\nUso:")
	for _, c := range commands {
		fmt.Fprintf(w, "  prazo %-8s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRode \"prazo <comando> -h\" para ver as flags.")
}

func newFlagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: prazo %s [flags]\n\n%s\n", name, usage)
		fs.PrintDefaults()
	}
	return fs
}
