package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"prazo/internal/doctor"
	"prazo/internal/logging"
	"prazo/internal/source/ical"
	"prazo/internal/source/webservice"
)

const doctorUsage = `Lê a URL do iCal de PRAZO_ICAL_URL. Nunca passe a URL como argumento: ela
contém o authtoken e ficaria no histórico do shell.`

func runDoctor(ctx context.Context, a *app, args []string) error {
	fs := newFlagSet("doctor", doctorUsage)
	icsFile := fs.String("ics", "", "ler um arquivo .ics local em vez de PRAZO_ICAL_URL")
	days := fs.Int("days", 30, "janela de próximos eventos (dias)")
	maxRows := fs.Int("max", 40, "máximo de linhas na tabela de próximos")
	probe := fs.Bool("probe-login", true, "testar /login/token.php com um usuário aleatório inexistente")
	user := fs.String("user", "", "usuário do Moodle: pede a senha e obtém um token temporário (nada é salvo)")
	pwStdin := fs.Bool("password-stdin", false, "com -user, lê a senha do stdin")
	skipWS := fs.Bool("skip-ws", false, "não consultar os web services")
	if err := fs.Parse(args); err != nil {
		return err
	}

	now := time.Now().UTC()
	feed, err := loadFeed(ctx, a, *icsFile)
	switch {
	case err != nil:
		fmt.Fprintf(a.stdout, "\n== iCal ==\n  ✗ %s\n", logging.Redact(err.Error()))
	case feed == nil:
		fmt.Fprintf(a.stdout, "\n== iCal ==\n  (PRAZO_ICAL_URL não definida; pulando)\n")
	default:
		doctor.AnalyzeICal(*feed, now, time.Duration(*days)*24*time.Hour).Print(a.stdout, a.cfg.Location, *maxRows)
	}
	if *skipWS {
		return nil
	}

	ws := webservice.NewClient(a.cfg.Moodle.BaseURL, a.http)
	token := a.cfg.Moodle.Token
	if token == "" && *user != "" {
		if token, err = login(ctx, ws, *user, *pwStdin); err != nil {
			fmt.Fprintf(a.stdout, "\n  ✗ %s\n%s", logging.Redact(err.Error()), loginHint(err))
		}
	}
	doctor.InspectWebService(ctx, ws, doctor.WebServiceOptions{ProbeLogin: *probe, Token: token, Now: now}).Print(a.stdout)
	return nil
}

func loadFeed(ctx context.Context, a *app, file string) (*ical.Feed, error) {
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		feed, err := ical.Parse(f)
		return &feed, err
	}
	if a.cfg.Moodle.ICalURL == "" {
		return nil, nil
	}
	client, err := ical.NewClient(a.cfg.Moodle.ICalURL, a.http)
	if err != nil {
		return nil, err
	}
	a.log.Info("baixando iCal", "url", logging.RedactURL(a.cfg.Moodle.ICalURL))
	feed, err := client.Fetch(ctx)
	return &feed, err
}

func loginHint(err error) string {
	if errors.Is(err, webservice.ErrServiceUnavailable) {
		return "    → a instância bloqueia o app; o Prazo vai usar só o iCal.\n"
	}
	return "    → contas que entram só pelo id.udesc/office365 (SSO) não funcionam aqui; veja docs/moodle-udesc.md.\n"
}

// login obtém um token temporário; a senha só existe em memória e é zerada após o uso.
func login(ctx context.Context, ws *webservice.Client, user string, fromStdin bool) (string, error) {
	pw, err := readPassword(user, fromStdin)
	if err != nil {
		return "", err
	}
	defer clear(pw)
	return ws.Login(ctx, user, string(pw))
}

func readPassword(user string, fromStdin bool) ([]byte, error) {
	if fromStdin {
		pw, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err != nil {
			return nil, fmt.Errorf("ler senha do stdin: %w", err)
		}
		return bytes.TrimRight(pw, "\r\n"), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New("-user exige um terminal interativo (ou use -password-stdin)")
	}
	fmt.Fprintf(os.Stderr, "Senha do Moodle para %s (não será salva): ", user)
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("ler senha: %w", err)
	}
	return pw, nil
}
