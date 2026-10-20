package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // binário independente do tzdata do sistema (imagem distroless)

	"prazo/internal/config"
	"prazo/internal/logging"
)

const usage = `prazo: sincroniza prazos do Moodle com o Google Agenda.

Uso:
  prazo doctor [flags]       diagnostica a exportação iCal e os web services do Moodle
  prazo sync --dry-run       lê o iCal, normaliza e mostra os próximos prazos

Rode "prazo <comando> -h" para ver as flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	log := logging.New(os.Stderr, cfg.LogFormat, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "doctor":
		err = runDoctor(ctx, cfg, log, os.Args[2:])
	case "sync":
		err = runSync(ctx, cfg, log, os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		log.Error("falhou", "cmd", os.Args[1], "err", err)
		os.Exit(1)
	}
}
