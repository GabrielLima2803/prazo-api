package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"prazo/internal/config"
	"prazo/internal/domain"
	"prazo/internal/httpx"
	"prazo/internal/source/ical"
	"prazo/internal/sync"
)

func runSync(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "só lê e normaliza; não escreve no Google")
	days := fs.Int("days", 30, "janela da tabela (dias a partir de agora)")
	links := fs.Bool("links", false, "mostrar a coluna de links")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if !*dryRun {
		return errors.New("integração com o Google ainda não existe (Fase 2); use --dry-run")
	}
	if cfg.ICalURL == "" {
		return errors.New("defina PRAZO_ICAL_URL (veja o README)")
	}

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	svc := &sync.Service{
		Source: &ical.Source{
			Fetcher: &ical.Fetcher{URL: cfg.ICalURL, Client: httpx.New(httpx.Options{MaxRetries: 3})},
			Log:     log,
		},
		Opts: domainOpts(cfg),
	}

	start := time.Now()
	all, fetched, err := svc.Plan(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	window := sync.Window(all, now, now.AddDate(0, 0, *days))
	log.Info("dry-run concluído",
		"buscados", fetched, "após_normalizar", len(all), "na_janela", len(window),
		"duração", time.Since(start).Round(time.Millisecond))

	fmt.Printf("%d prazo(s) nos próximos %d dias (%d eventos no iCal, %d após agrupar/filtrar)\n\n",
		len(window), *days, fetched, len(all))
	sync.PrintTable(os.Stdout, window, now, loc, *links)
	return nil
}

func domainOpts(cfg config.Config) domain.NormalizeOptions {
	return domain.NormalizeOptions{Filter: cfg.Filter, IncludeOpenings: cfg.IncludeOpenings}
}
