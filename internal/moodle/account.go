package moodle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"
)

type Method string

const (
	MethodLogin Method = "login" // usuário e senha → token → URL do iCal montada pelo Prazo
	MethodICal  Method = "ical"  // URL colada pelo usuário (contas só com SSO)
	MethodEnv   Method = "env"   // PRAZO_ICAL_URL, sem conta salva (uso pelo terminal)
)

// Account é a conta salva. ICalURL e Token são segredos e nunca saem pela API.
type Account struct {
	Method      Method    `json:"method"`
	BaseURL     string    `json:"baseUrl"`
	ICalURL     string    `json:"icalUrl"`
	Token       string    `json:"token,omitempty"`
	UserID      int64     `json:"userId,omitempty"`
	FullName    string    `json:"fullName,omitempty"`
	SiteName    string    `json:"siteName,omitempty"`
	ConnectedAt time.Time `json:"connectedAt"`
	// NeedsReauth: o Moodle recusou a URL do iCal (senha trocada). Fica
	// marcado até o próximo login, para não bater no Moodle à toa.
	NeedsReauth bool `json:"needsReauth,omitempty"`
}

// Host é o host do Moodle em minúsculas, parte da identidade do usuário.
func (m Account) Host() string { return hostOf(m.BaseURL) }

// Authenticator descobre quem é a pessoa no Moodle, sem guardar nada.
type Authenticator struct {
	http    *httpx.Client
	allowed collections.Set[string] // hosts aceitos; vazio = qualquer um
}

// AccountService é a conta de um usuário, no KV dele.
type AccountService struct {
	kv      storage.KV // nil: só a variável de ambiente
	http    *httpx.Client
	log     *slog.Logger
	envICal string
	now     func() time.Time
}

var (
	ErrNotConnected      = errors.New("nenhuma conta do Moodle conectada")
	ErrReauthRequired    = fmt.Errorf("%w: a senha do Moodle mudou; entre de novo pelo app", ErrUnauthorized)
	ErrInvalidBaseURL    = errors.New("o endereço do Moodle precisa começar com https://")
	ErrHostNotAllowed    = errors.New("este Moodle não é atendido por este servidor do Prazo")
	ErrExportUnavailable = errors.New("este Moodle não libera a exportação do calendário pelo app; cole a URL do iCal")
	ErrEmptyFeed         = errors.New("o calendário exportado veio vazio; confira se é a conta certa")
	ErrMissingUserID     = errors.New("a URL do calendário não tem o userid; gere de novo em Calendário → Exportar")
)

func NewAccountService(kv storage.KV, http *httpx.Client, log *slog.Logger, envICalURL string) *AccountService {
	return &AccountService{kv: kv, http: http, log: log, envICal: envICalURL, now: time.Now}
}

// Moodle devolve a conta salva ou, sem ela, a da variável de ambiente.
func (s *AccountService) Moodle(ctx context.Context) (Account, error) {
	if s.kv != nil {
		b, err := s.kv.Get(ctx, storage.KeyMoodle)
		switch {
		case err == nil:
			var m Account
			if err := json.Unmarshal(b, &m); err != nil {
				return Account{}, fmt.Errorf("conta do Moodle corrompida: %w", err)
			}
			return m, nil
		case !errors.Is(err, storage.ErrNotFound):
			return Account{}, err
		}
	}
	if s.envICal == "" {
		return Account{}, ErrNotConnected
	}
	c, err := NewICalClient(s.envICal, s.http)
	if err != nil {
		return Account{}, err
	}
	return Account{Method: MethodEnv, BaseURL: c.BaseURL(), ICalURL: s.envICal}, nil
}

// Save guarda a conta só depois de baixar o calendário com sucesso: uma conta
// salva sempre funcionou pelo menos uma vez. Devolve quantos eventos vieram.
func (s *AccountService) Save(ctx context.Context, m Account) (int, error) {
	if s.kv == nil {
		return 0, errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	c, err := NewICalClient(m.ICalURL, s.http)
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

func (s *AccountService) Disconnect(ctx context.Context) error {
	if s.kv == nil {
		return errors.New("sem armazenamento: defina PRAZO_MASTER_KEY")
	}
	return s.kv.Delete(ctx, storage.KeyMoodle)
}

// Source lê a conta a cada busca: entrar de novo pelo app vale na hora.
func (s *AccountService) Source() Source {
	return SourceFunc(func(ctx context.Context) (Source, error) {
		m, err := s.Moodle(ctx)
		if err != nil {
			return nil, err
		}
		if m.NeedsReauth {
			return nil, ErrReauthRequired
		}
		c, err := NewICalClient(m.ICalURL, s.http)
		if err != nil {
			return nil, err
		}
		return &watchedSource{Source: NewICalSource(c, s.log), svc: s, account: m}, nil
	})
}

// watchedSource marca a conta quando o Moodle recusa a credencial e completa
// o status de entrega pelos web services, quando há token.
type watchedSource struct {
	Source
	svc     *AccountService
	account Account
}

func (w *watchedSource) Fetch(ctx context.Context) ([]domain.Deadline, error) {
	d, err := w.Source.Fetch(ctx)
	if errors.Is(err, ErrUnauthorized) && w.account.Method != MethodEnv {
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

func (s *AccountService) markReauth(ctx context.Context, m Account) error {
	m.NeedsReauth = true
	return s.put(ctx, m)
}

func (s *AccountService) put(ctx context.Context, m Account) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.kv.Put(ctx, storage.KeyMoodle, b)
}
