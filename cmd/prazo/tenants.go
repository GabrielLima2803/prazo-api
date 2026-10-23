package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"
)

// manager abre o armazenamento e migra uma instalação antiga de um usuário só.
func (a *app) manager(ctx context.Context) (*tenant.Manager, error) {
	kv, err := a.secrets()
	if err != nil {
		return nil, err
	}
	m := tenant.NewManager(kv, a.buildTenant, a.log, a.cfg.Server.MaxUsers)
	if _, _, err := m.MigrateLegacy(ctx, a.cfg.Moodle.ICalURL); err != nil {
		return nil, fmt.Errorf("migrar dados antigos: %w", err)
	}
	return m, nil
}

// buildTenant monta as dependências de um usuário em cima do KV dele.
func (a *app) buildTenant(u tenant.User, kv store.KV) *tenant.Tenant {
	log := a.log.With("user", u.ID)
	accounts := account.NewService(kv, a.http, log, "")
	svc := sync.NewService(accounts.Source(), a.cfg.Normalize, sync.WithLogger(log))
	return &tenant.Tenant{
		User:     u,
		KV:       kv,
		Accounts: accounts,
		Prefs:    prefs.NewStore(kv),
		Runner:   sync.NewRunner(svc, a.target(kv), log),
	}
}

// cliTenant é o usuário dos comandos de terminal (sync, auth) e do
// PRAZO_API_TOKEN: o da PRAZO_ICAL_URL ou, sem ela, o único cadastrado.
func (a *app) cliTenant(ctx context.Context, m *tenant.Manager) (*tenant.Tenant, error) {
	if env := a.cfg.Moodle.ICalURL; env != "" {
		host, userID := account.IdentityFromICal(env)
		if userID == 0 {
			return nil, account.ErrMissingUserID
		}
		t, err := m.Register(ctx, tenant.User{ID: tenant.IDFor(host, userID), Host: host, MoodleUserID: userID})
		if err != nil {
			return nil, err
		}
		if _, err := t.Accounts.Moodle(ctx); errors.Is(err, account.ErrNotConnected) {
			acc := account.Moodle{Method: account.MethodICal, BaseURL: "https://" + host, ICalURL: env, UserID: userID}
			b, _ := json.Marshal(acc)
			if err := t.KV.Put(ctx, store.KeyMoodle, b); err != nil {
				return nil, err
			}
		}
		return t, nil
	}
	users, err := m.Users(ctx)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, fmt.Errorf("%d usuários cadastrados: defina PRAZO_ICAL_URL para escolher qual usar no terminal", len(users))
	}
	return m.Get(ctx, users[0].ID)
}
