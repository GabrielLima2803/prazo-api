package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"

	"golang.org/x/term"

	"prazo/internal/config"
	"prazo/internal/doctor"
	"prazo/internal/domain"
	"prazo/internal/httpx"
	"prazo/internal/logging"
	"prazo/internal/source/ical"
	"prazo/internal/source/webservice"
)

func runDoctor(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	icsFile := fs.String("ics", "", "ler um arquivo .ics local em vez de PRAZO_ICAL_URL")
	days := fs.Int("days", 30, "janela de próximos eventos (dias)")
	maxRows := fs.Int("max", 40, "máximo de linhas na tabela de próximos")
	probe := fs.Bool("probe-login", true, "testar se /login/token.php aceita login (usa um usuário aleatório inexistente)")
	user := fs.String("user", "", "usuário do Moodle: pede a senha, obtém um token temporário e lista as funções (nada é salvo)")
	pwStdin := fs.Bool("password-stdin", false, "com -user, lê a senha do stdin (para scripts; ex.: printf %s \"$SENHA\" | prazo doctor -user X -password-stdin)")
	skipWS := fs.Bool("skip-ws", false, "não consultar os web services")
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Uso: prazo doctor [flags]

Lê a URL do iCal de PRAZO_ICAL_URL (nunca passe a URL como argumento: ela
contém o authtoken e ficaria no histórico do shell).

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	hc := httpx.New(httpx.Options{Timeout: 30 * time.Second, MaxRetries: 3, Concurrency: 1})
	out := os.Stdout

	// 1. iCal
	events, skipped, err := loadICal(ctx, cfg, hc, log, *icsFile)
	switch {
	case err != nil:
		fmt.Fprintf(out, "\n== iCal ==\n  ✗ %s\n", logging.Redact(err.Error()))
	case events == nil && skipped == nil:
		fmt.Fprintf(out, "\n== iCal ==\n  (PRAZO_ICAL_URL não definida e -ics não informado; pulando)\n")
	default:
		r := doctor.Analyze(events, skipped, time.Now().UTC(), time.Duration(*days)*24*time.Hour)
		r.Print(out, loc, *maxRows)
	}

	if *skipWS {
		return nil
	}

	// 2. Web services
	ws := &webservice.Client{BaseURL: cfg.MoodleURL, HTTP: hc}
	fmt.Fprintf(out, "\n== Web services (%s) ==\n", cfg.MoodleURL)

	pc, err := ws.PublicConfig(ctx)
	if err != nil {
		fmt.Fprintf(out, "  ✗ config pública: %s\n", logging.Redact(err.Error()))
	} else {
		fmt.Fprintf(out, "  site:               %s\n", pc.SiteName)
		fmt.Fprintf(out, "  idioma padrão:      %s\n", pc.Lang)
		fmt.Fprintf(out, "  web services:       %s\n", yesNo(pc.EnableWebServices == 1))
		fmt.Fprintf(out, "  serviço mobile:     %s\n", yesNo(pc.EnableMobileService == 1))
		fmt.Fprintf(out, "  tipo de login app:  %s\n", pc.LoginTypeName())
		fmt.Fprintf(out, "  formulário login:   %s\n", yesNo(pc.ShowLoginForm == 1))
		for _, idp := range pc.IdentityProviders {
			fmt.Fprintf(out, "  provedor SSO:       %s\n", idp.Name)
		}
		if pc.MaintenanceEnabled == 1 {
			fmt.Fprintf(out, "  ⚠ site em manutenção\n")
		}
	}

	if *probe {
		p, err := ws.ProbeTokenEndpoint(ctx)
		switch {
		case err != nil:
			fmt.Fprintf(out, "  ✗ /login/token.php: %s\n", logging.Redact(err.Error()))
		case p.Available:
			fmt.Fprintf(out, "  ✓ /login/token.php ativo (respondeu %q a um usuário inexistente)\n", p.ErrorCode)
		default:
			fmt.Fprintf(out, "  ✗ /login/token.php bloqueado: %s (%s)\n", p.Message, p.ErrorCode)
		}
	}

	token := cfg.MoodleToken
	if token == "" && *user != "" {
		token, err = login(ctx, ws, *user, *pwStdin)
		if err != nil {
			fmt.Fprintf(out, "  ✗ login: %s\n", logging.Redact(err.Error()))
			if errors.Is(err, webservice.ErrServiceUnavailable) {
				fmt.Fprintf(out, "    → a instância bloqueia o app; o Prazo vai usar só o iCal.\n")
			} else {
				fmt.Fprintf(out, "    → se sua conta entra pelo id.udesc/office365 (SSO), a senha não funciona aqui;\n")
				fmt.Fprintf(out, "      veja a seção \"Token via SSO\" no README.\n")
			}
		}
	}
	if token == "" {
		fmt.Fprintf(out, "  (sem token: defina PRAZO_MOODLE_TOKEN ou use -user para listar as funções)\n")
		return nil
	}
	return reportFunctions(ctx, out, ws, token)
}

func loadICal(ctx context.Context, cfg config.Config, hc *httpx.Client, log *slog.Logger, file string) ([]ical.Event, []error, error) {
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		return ical.Parse(f)
	}
	if cfg.ICalURL == "" {
		return nil, nil, nil
	}
	log.Info("baixando iCal", "url", logging.RedactURL(cfg.ICalURL))
	f := &ical.Fetcher{URL: cfg.ICalURL, Client: hc}
	return f.Fetch(ctx)
}

// login obtém um token temporário. A senha só existe em memória e é zerada
// logo após o uso.
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
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("-user exige um terminal interativo (ou use -password-stdin)")
	}
	fmt.Fprintf(os.Stderr, "Senha do Moodle para %s (não será salva): ", user)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("ler senha: %w", err)
	}
	return pw, nil
}

func reportFunctions(ctx context.Context, out io.Writer, ws *webservice.Client, token string) error {
	si, err := ws.SiteInfo(ctx, token)
	if err != nil {
		fmt.Fprintf(out, "  ✗ site info: %s\n", logging.Redact(err.Error()))
		return nil
	}
	fmt.Fprintf(out, "  moodle:             %s (versão %s)\n", si.Release, si.Version)
	fmt.Fprintf(out, "  userid:             %d\n", si.UserID)
	fmt.Fprintf(out, "  funções liberadas:  %d\n", len(si.Functions))

	set := si.FunctionSet()
	for _, fn := range webservice.WantedFunctions {
		mark := "✗"
		if _, ok := set[fn]; ok {
			mark = "✓"
		}
		fmt.Fprintf(out, "    %s %s\n", mark, fn)
	}

	if _, ok := set["core_enrol_get_users_courses"]; ok {
		var courses []struct {
			ShortName string `json:"shortname"`
			FullName  string `json:"fullname"`
		}
		args := url.Values{"userid": {strconv.FormatInt(si.UserID, 10)}}
		if err := ws.Call(ctx, token, "core_enrol_get_users_courses", args, &courses); err != nil {
			fmt.Fprintf(out, "  ✗ disciplinas: %s\n", logging.Redact(err.Error()))
		} else {
			fmt.Fprintf(out, "\n  disciplinas matriculadas: %d\n", len(courses))
			for _, c := range courses {
				fmt.Fprintf(out, "    [%s] %s  (%s)\n", domain.CourseCode(c.ShortName), c.FullName, c.ShortName)
			}
		}
	}

	if _, ok := set["core_calendar_get_action_events_by_timesort"]; ok {
		var resp struct {
			Events []struct {
				ID         int64  `json:"id"`
				Name       string `json:"name"`
				ModuleName string `json:"modulename"`
				EventType  string `json:"eventtype"`
				TimeSort   int64  `json:"timesort"`
				Course     struct {
					ShortName string `json:"shortname"`
				} `json:"course"`
				URL string `json:"url"`
			} `json:"events"`
		}
		args := url.Values{
			"timesortfrom": {strconv.FormatInt(time.Now().Unix(), 10)},
			"limitnum":     {"50"},
		}
		if err := ws.Call(ctx, token, "core_calendar_get_action_events_by_timesort", args, &resp); err != nil {
			fmt.Fprintf(out, "  ✗ action events: %s\n", logging.Redact(err.Error()))
			return nil
		}
		fmt.Fprintf(out, "\n  action events futuros (ainda não entregues): %d\n", len(resp.Events))
		for _, e := range resp.Events {
			fmt.Fprintf(out, "    id=%d mod=%s tipo=%s curso=%s nome=%q\n",
				e.ID, e.ModuleName, e.EventType, e.Course.ShortName, e.Name)
		}
		if len(resp.Events) > 0 {
			fmt.Fprintf(out, "    (o id acima deve bater com o número do UID do iCal: %d@...)\n", resp.Events[0].ID)
		}
	}
	return nil
}

func yesNo(b bool) string {
	if b {
		return "sim"
	}
	return "não"
}
