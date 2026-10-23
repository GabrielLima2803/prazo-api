package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
)

func runSync(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("sync", "Lê o iCal do Moodle, normaliza e aplica os prazos na agenda do Google.")
	dryRun := fs.Bool("dry-run", false, "só lê e mostra a tabela; não escreve no Google")
	days := fs.Int("days", 30, "janela da tabela do --dry-run (dias a partir de agora)")
	links := fs.Bool("links", false, "mostrar a coluna de links no --dry-run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, err := a.syncService()
	if err != nil {
		return err
	}
	if *dryRun {
		return dryRunSync(ctx, a, svc, *days, *links)
	}

	kv, err := a.secrets()
	if err != nil {
		return err
	}
	t, err := a.target(kv)(ctx)
	if err != nil {
		return err
	}

	start := time.Now()
	r, err := svc.Sync(ctx, t)
	if err != nil {
		return err
	}
	if r.Err != nil {
		a.log.Warn("alguns eventos falharam", "falhas", r.Failed, "err", r.Err)
	}
	a.log.Info("sync concluído",
		"buscados", r.Plan.Fetched, "prazos", len(r.Plan.Deadlines),
		"criados", r.Created, "atualizados", r.Updated, "ocultos", r.Hidden, "falhas", r.Failed,
		"duracao", time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(a.stdout, "%d prazo(s) na agenda %q: %d criado(s), %d atualizado(s), %d falha(s).\n",
		len(r.Plan.Deadlines), a.cfg.Google.CalendarName, r.Created, r.Updated, r.Failed)
	return nil
}

func (a *app) syncService() (*sync.Service, error) {
	if a.cfg.Moodle.ICalURL == "" {
		return nil, errors.New("defina PRAZO_ICAL_URL (veja docs/desenvolvimento.md)")
	}
	client, err := ical.NewClient(a.cfg.Moodle.ICalURL, a.http)
	if err != nil {
		return nil, err
	}
	return sync.NewService(ical.NewSource(client, a.log), a.cfg.Normalize), nil
}

func dryRunSync(ctx context.Context, a *app, svc *sync.Service, days int, links bool) error {
	start := time.Now()
	plan, err := svc.Plan(ctx)
	if err != nil {
		return err
	}
	upcoming := plan.Upcoming(time.Duration(days) * 24 * time.Hour)
	a.log.Info("dry-run concluído",
		"buscados", plan.Fetched, "normalizados", len(plan.Deadlines), "na_janela", len(upcoming),
		"duracao", time.Since(start).Round(time.Millisecond))

	fmt.Fprintf(a.stdout, "%d prazo(s) nos próximos %d dias (%d eventos no iCal, %d seriam sincronizados)\n\n",
		len(upcoming), days, plan.Fetched, len(plan.Deadlines))
	printDeadlines(a.stdout, upcoming, plan.At, a.cfg.Location, links)
	return nil
}
