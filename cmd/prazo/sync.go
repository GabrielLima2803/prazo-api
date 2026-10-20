package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"prazo/internal/source/ical"
	"prazo/internal/sync"
)

var errGoogleNotImplemented = errors.New("integração com o Google ainda não existe (Fase 2); use --dry-run")

func runSync(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("sync", "Lê o iCal, normaliza e (na Fase 2) aplica no Google Agenda.")
	dryRun := fs.Bool("dry-run", false, "só lê e normaliza; não escreve no Google")
	days := fs.Int("days", 30, "janela da tabela (dias a partir de agora)")
	links := fs.Bool("links", false, "mostrar a coluna de links")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*dryRun {
		return errGoogleNotImplemented
	}
	if a.cfg.Moodle.ICalURL == "" {
		return errors.New("defina PRAZO_ICAL_URL (veja o README)")
	}

	client, err := ical.NewClient(a.cfg.Moodle.ICalURL, a.http)
	if err != nil {
		return err
	}
	svc := sync.NewService(ical.NewSource(client, a.log), a.cfg.Normalize)

	start := time.Now()
	plan, err := svc.Plan(ctx)
	if err != nil {
		return err
	}
	upcoming := plan.Upcoming(time.Duration(*days) * 24 * time.Hour)
	a.log.Info("dry-run concluído",
		"buscados", plan.Fetched, "normalizados", len(plan.Deadlines), "na_janela", len(upcoming),
		"duracao", time.Since(start).Round(time.Millisecond))

	fmt.Fprintf(a.stdout, "%d prazo(s) nos próximos %d dias (%d eventos no iCal, %d após agrupar/filtrar)\n\n",
		len(upcoming), *days, plan.Fetched, len(plan.Deadlines))
	printDeadlines(a.stdout, upcoming, plan.At, a.cfg.Location, *links)
	return nil
}
