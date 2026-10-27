package main

import (
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"

	"context"
	"fmt"
	"time"
)

func runSync(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("sync", "Lê o iCal do Moodle, normaliza e aplica os prazos na agenda do Google.")
	dryRun := fs.Bool("dry-run", false, "só lê e mostra a tabela; não escreve no Google")
	days := fs.Int("days", 30, "janela da tabela do --dry-run (dias a partir de agora)")
	links := fs.Bool("links", false, "mostrar a coluna de links no --dry-run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun && a.cfg.Storage.MasterKey == nil {
		// Sem armazenamento: só a PRAZO_ICAL_URL, sem conta salva.
		acc := moodle.NewAccountService(nil, a.http, a.log, a.cfg.Moodle.ICalURL)
		return dryRunSync(ctx, a, sync.NewService(acc.Source(), a.cfg.Normalize), *days, *links)
	}
	m, err := a.manager(ctx)
	if err != nil {
		return err
	}
	tn, err := a.cliTenant(ctx, m)
	if err != nil {
		return err
	}
	svc := sync.NewService(tn.Accounts.Source(), a.cfg.Normalize, sync.WithLogger(a.log))
	if *dryRun {
		return dryRunSync(ctx, a, svc, *days, *links)
	}
	t, err := a.target(tn.KV)(ctx)
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
		"criados", r.Created, "atualizados", r.Updated, "iguais", r.Unchanged, "apagados", r.Deleted,
		"ocultos", r.Hidden, "falhas", r.Failed, "duracao", time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(a.stdout, "%d prazo(s) na agenda %q: %d criado(s), %d atualizado(s), %d sem mudança, %d apagado(s), %d falha(s).\n",
		len(r.Plan.Deadlines), a.cfg.Google.CalendarName, r.Created, r.Updated, r.Unchanged, r.Deleted, r.Failed)
	if r.DeleteBlocked > 0 {
		fmt.Fprintf(a.stdout, "Atenção: %d prazo(s) sumiram de uma vez do Moodle; nada foi apagado por segurança.\n", r.DeleteBlocked)
	}
	return nil
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
