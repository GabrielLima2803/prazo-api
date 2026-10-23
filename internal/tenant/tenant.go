// Package tenant organiza os usuários do servidor: cada aluno tem um espaço
// próprio no KV (conta do Moodle, token do Google, preferências, estado da
// sincronização) e um Runner. O Manager registra usuários e sincroniza todos,
// um de cada vez, para não sobrecarregar o Moodle nem o Google.
package tenant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
)

var (
	ErrUnknownUser  = errors.New("usuário desconhecido")
	ErrTooManyUsers = errors.New("este servidor do Prazo atingiu o limite de usuários")
)

// User é o que o servidor sabe de cada aluno fora do espaço dele.
type User struct {
	ID           string    `json:"id"`
	Host         string    `json:"host"`
	MoodleUserID int64     `json:"moodleUserId"`
	Name         string    `json:"name,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// IDFor é estável e não revela o userid do Moodle nos nomes dos arquivos.
func IDFor(host string, moodleUserID int64) string {
	sum := sha256.Sum256([]byte(strings.ToLower(host) + "|" + strconv.FormatInt(moodleUserID, 10)))
	return hex.EncodeToString(sum[:8])
}

// Tenant é um usuário com as dependências já montadas.
type Tenant struct {
	User     User
	KV       store.KV
	Accounts *account.Service
	Prefs    *prefs.Store
	Runner   *sync.Runner
}

// Builder monta as dependências de um usuário a partir do KV dele; fica no
// cmd, que conhece Google, HTTP e configuração.
type Builder func(u User, kv store.KV) *Tenant

type Manager struct {
	root     store.KV
	build    Builder
	log      *slog.Logger
	maxUsers int
	now      func() time.Time

	mu      gosync.Mutex
	tenants map[string]*Tenant
}

func NewManager(root store.KV, build Builder, log *slog.Logger, maxUsers int) *Manager {
	return &Manager{root: root, build: build, log: log, maxUsers: maxUsers, now: time.Now, tenants: map[string]*Tenant{}}
}

// Namespace é o KV de um usuário.
func (m *Manager) Namespace(id string) store.KV { return store.Namespace(m.root, "u"+id+"_") }

func (m *Manager) Users(ctx context.Context) ([]User, error) {
	b, err := m.root.Get(ctx, store.KeyUsers)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var users []User
	return users, json.Unmarshal(b, &users)
}

// Register cria o usuário se ainda não existir (e atualiza o nome) e devolve
// o Tenant dele.
func (m *Manager) Register(ctx context.Context, u User) (*Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	users, err := m.Users(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(users, func(x User) bool { return x.ID == u.ID })
	switch {
	case i >= 0:
		if u.Name != "" {
			users[i].Name = u.Name
		}
		u = users[i]
	case m.maxUsers > 0 && len(users) >= m.maxUsers:
		return nil, ErrTooManyUsers
	default:
		u.CreatedAt = m.now().UTC()
		users = append(users, u)
		m.log.Info("novo usuário", "user", u.ID)
	}
	if err := m.saveUsers(ctx, users); err != nil {
		return nil, err
	}
	return m.tenantLocked(u), nil
}

func (m *Manager) Get(ctx context.Context, id string) (*Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tenants[id]; ok {
		return t, nil
	}
	users, err := m.Users(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(users, func(x User) bool { return x.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownUser, id)
	}
	return m.tenantLocked(users[i]), nil
}

func (m *Manager) tenantLocked(u User) *Tenant {
	if t, ok := m.tenants[u.ID]; ok {
		t.User = u
		return t
	}
	t := m.build(u, m.Namespace(u.ID))
	m.tenants[u.ID] = t
	return t
}

func (m *Manager) saveUsers(ctx context.Context, users []User) error {
	b, err := json.Marshal(users)
	if err != nil {
		return err
	}
	return m.root.Put(ctx, store.KeyUsers, b)
}

// Loop sincroniza todos os usuários, um de cada vez, e espera interval (±10%)
// até a próxima rodada. Usuários novos entram na rodada seguinte; o login já
// dispara a primeira sincronização dele.
func (m *Manager) Loop(ctx context.Context, interval time.Duration) {
	for {
		users, err := m.Users(ctx)
		if err != nil {
			m.log.Error("listar usuários", "err", err)
		}
		for _, u := range users {
			t, err := m.Get(ctx, u.ID)
			if err != nil {
				continue
			}
			if _, err := t.Runner.RunNow(ctx); errors.Is(err, context.Canceled) {
				return
			}
		}
		wait := jitter(interval)
		next := m.now().Add(wait).UTC()
		for _, u := range users {
			if t, err := m.Get(ctx, u.ID); err == nil {
				t.Runner.SetNextRun(next)
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func jitter(d time.Duration) time.Duration {
	spread := int64(d) / 10
	if spread <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(2*spread)-spread)
}
