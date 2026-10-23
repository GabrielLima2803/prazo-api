package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/source"
	"github.com/GabrielLima2803/prazo-sync/internal/source/ical"
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

// Save guarda a conta só depois de baixar o calendário com sucesso: uma conta
// salva sempre funcionou pelo menos uma vez. Devolve quantos eventos vieram.
func (s *Service) Save(ctx context.Context, m Moodle) (int, error) {
	if s.kv == nil {
		return 0, errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	c, err := ical.NewClient(m.ICalURL, s.http)
	if err != nil {
		return 0, err
	}
	feed, err := c.Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("testar o calendário: %w", err)
	}
	if len(feed.Events) == 0 {
		return 0, ErrEmptyFeed
	}
	m.ConnectedAt = s.now().UTC()
	m.NeedsReauth = false
	return len(feed.Events), s.put(ctx, m)
}

func (s *Service) Disconnect(ctx context.Context) error {
	if s.kv == nil {
		return errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	return s.kv.Delete(ctx, store.KeyMoodle)
}

// Source lê a conta a cada busca: entrar de novo pelo app vale na hora.
func (s *Service) Source() source.Source {
	return source.Func(func(ctx context.Context) (source.Source, error) {
		m, err := s.Moodle(ctx)
		if err != nil {
			return nil, err
		}
		if m.NeedsReauth {
			return nil, ErrReauthRequired
		}
		c, err := ical.NewClient(m.ICalURL, s.http)
		if err != nil {
			return nil, err
		}
		return &watchedSource{Source: ical.NewSource(c, s.log), svc: s, account: m}, nil
	})
}

// watchedSource marca a conta quando o Moodle recusa a credencial e completa
// o status de entrega pelos web services, quando há token.
type watchedSource struct {
	source.Source
	svc     *Service
	account Moodle
}

func (w *watchedSource) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	d, err := w.Source.Fetch(ctx)
	if errors.Is(err, source.ErrUnauthorized) && w.account.Method != MethodEnv {
		if markErr := w.svc.markReauth(ctx, w.account); markErr != nil {
			w.svc.log.Error("marcar conta para novo login", "err", markErr)
		}
		w.svc.log.Warn("o Moodle recusou a URL do calendário; o usuário precisa entrar de novo")
		return nil, ErrReauthRequired
	}
	if err != nil {
		return nil, err
	}
	w.svc.markSubmitted(ctx, w.account, d)
	return d, nil
}

func (s *Service) markReauth(ctx context.Context, m Moodle) error {
	m.NeedsReauth = true
	return s.put(ctx, m)
}

func (s *Service) put(ctx context.Context, m Moodle) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.kv.Put(ctx, store.KeyMoodle, b)
}
