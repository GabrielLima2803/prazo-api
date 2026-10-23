package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/source"
	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
	"github.com/GabrielLima2803/prazo-sync/internal/source/webservice"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

func NewService(kv store.KV, http *httpx.Client, log *slog.Logger, envICalURL string) *Service {
	return &Service{kv: kv, http: http, log: log, envICal: envICalURL, now: time.Now}
}

// Moodle devolve a conta salva ou, sem ela, a da variável de ambiente.
func (s *Service) Moodle(ctx context.Context) (Moodle, error) {
	if s.kv != nil {
		b, err := s.kv.Get(ctx, store.KeyMoodle)
		switch {
		case err == nil:
			var m Moodle
			if err := json.Unmarshal(b, &m); err != nil {
				return Moodle{}, fmt.Errorf("conta do Moodle corrompida: %w", err)
			}
			return m, nil
		case !errors.Is(err, store.ErrNotFound):
			return Moodle{}, err
		}
	}
	if s.envICal == "" {
		return Moodle{}, ErrNotConnected
	}
	c, err := ical.NewClient(s.envICal, s.http)
	if err != nil {
		return Moodle{}, err
	}
	return Moodle{Method: MethodEnv, BaseURL: c.BaseURL(), ICalURL: s.envICal}, nil
}

// Source lê a conta a cada busca: conectar ou trocar a conta pelo app vale
// na hora.
func (s *Service) Source() source.Source {
	return source.Func(func(ctx context.Context) (source.Source, error) {
		m, err := s.Moodle(ctx)
		if err != nil {
			return nil, err
		}
		c, err := ical.NewClient(m.ICalURL, s.http)
		if err != nil {
			return nil, err
		}
		return ical.NewSource(c, s.log), nil
	})
}

// ConnectWithLogin troca usuário e senha por um token de web service, pede ao
// Moodle o authtoken da exportação e monta a URL do iCal. A senha só é usada
// nessa chamada.
func (s *Service) ConnectWithLogin(ctx context.Context, baseURL, username, password string) (Moodle, int, error) {
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return Moodle{}, 0, err
	}
	ws := webservice.NewClient(base, s.http)
	token, err := ws.Login(ctx, strings.TrimSpace(username), password)
	if err != nil {
		return Moodle{}, 0, err
	}
	info, err := ws.SiteInfo(ctx, token)
	if err != nil {
		return Moodle{}, 0, fmt.Errorf("dados do usuário: %w", err)
	}
	if !info.FunctionSet().Has("core_calendar_get_calendar_export_token") {
		return Moodle{}, 0, ErrExportUnavailable
	}
	exportToken, err := ws.CalendarExportToken(ctx, token)
	if err != nil {
		return Moodle{}, 0, fmt.Errorf("%w (%w)", ErrExportUnavailable, err)
	}
	m := Moodle{
		Method:   MethodLogin,
		BaseURL:  base,
		ICalURL:  exportURL(base, info.UserID, exportToken),
		Token:    token,
		UserID:   info.UserID,
		FullName: info.FullName,
		SiteName: info.SiteName,
	}
	return s.save(ctx, m)
}

// ConnectWithICal salva uma URL de exportação colada pelo usuário, depois de
// conferir que ela devolve um calendário.
func (s *Service) ConnectWithICal(ctx context.Context, icalURL string) (Moodle, int, error) {
	c, err := ical.NewClient(strings.TrimSpace(icalURL), s.http)
	if err != nil {
		return Moodle{}, 0, err
	}
	return s.save(ctx, Moodle{Method: MethodICal, BaseURL: c.BaseURL(), ICalURL: strings.TrimSpace(icalURL)})
}

func (s *Service) Disconnect(ctx context.Context) error {
	if s.kv == nil {
		return errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	return s.kv.Delete(ctx, store.KeyMoodle)
}

// save só grava depois de buscar o calendário com sucesso: uma conta salva
// sempre funcionou pelo menos uma vez.
func (s *Service) save(ctx context.Context, m Moodle) (Moodle, int, error) {
	if s.kv == nil {
		return Moodle{}, 0, errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	c, err := ical.NewClient(m.ICalURL, s.http)
	if err != nil {
		return Moodle{}, 0, err
	}
	feed, err := c.Fetch(ctx)
	if err != nil {
		return Moodle{}, 0, fmt.Errorf("testar o calendário: %w", err)
	}
	if len(feed.Events) == 0 {
		return Moodle{}, 0, ErrEmptyFeed
	}
	m.ConnectedAt = s.now().UTC()
	b, err := json.Marshal(m)
	if err != nil {
		return Moodle{}, 0, err
	}
	if err := s.kv.Put(ctx, store.KeyMoodle, b); err != nil {
		return Moodle{}, 0, err
	}
	return m, len(feed.Events), nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", ErrInvalidBaseURL
	}
	return "https://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

// exportURL reproduz a URL de "Exportar calendário → Todos os eventos".
func exportURL(base string, userID int64, authToken string) string {
	q := url.Values{
		"userid":      {strconv.FormatInt(userID, 10)},
		"authtoken":   {authToken},
		"preset_what": {"all"},
		"preset_time": {"custom"},
	}
	return base + "/calendar/export_execute.php?" + q.Encode()
}
