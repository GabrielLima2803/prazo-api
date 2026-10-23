package tenant

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

// Chaves da instalação de um usuário só, antes dos namespaces.
var legacyKeys = []string{
	store.KeyGoogleToken, store.KeyGoogleCalendar, store.KeyPreferences,
	store.KeyMoodle, store.KeySyncState, store.KeyPendingSeen,
}

// MigrateLegacy leva a instalação de um usuário só para o espaço dele: sem
// isso, o primeiro login criaria uma agenda nova e duplicaria os eventos. A
// identidade vem da conta salva ou da PRAZO_ICAL_URL. Não faz nada se já
// existirem usuários ou se não houver o que migrar.
func (m *Manager) MigrateLegacy(ctx context.Context, envICal string) (User, bool, error) {
	users, err := m.Users(ctx)
	if err != nil || len(users) > 0 {
		return User{}, false, err
	}

	var acc account.Moodle
	hasAccount := false
	if b, err := m.root.Get(ctx, store.KeyMoodle); err == nil {
		hasAccount = json.Unmarshal(b, &acc) == nil && acc.UserID != 0
	} else if !errors.Is(err, store.ErrNotFound) {
		return User{}, false, err
	}

	u := User{Name: acc.FullName}
	switch {
	case hasAccount:
		u.Host, u.MoodleUserID = acc.Host(), acc.UserID
	case envICal != "":
		u.Host, u.MoodleUserID = account.IdentityFromICal(envICal)
	}
	if u.MoodleUserID == 0 {
		return User{}, false, nil
	}
	u.ID = IDFor(u.Host, u.MoodleUserID)

	ns := m.Namespace(u.ID)
	for _, key := range legacyKeys {
		b, err := m.root.Get(ctx, key)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return User{}, false, err
		}
		if err := ns.Put(ctx, key, b); err != nil {
			return User{}, false, err
		}
	}
	if !hasAccount {
		c := account.Moodle{Method: account.MethodICal, BaseURL: "https://" + u.Host, ICalURL: envICal, UserID: u.MoodleUserID}
		b, _ := json.Marshal(c)
		if err := ns.Put(ctx, store.KeyMoodle, b); err != nil {
			return User{}, false, err
		}
	}
	if _, err := m.Register(ctx, u); err != nil {
		return User{}, false, err
	}
	// Sessões antigas não sabem de qual usuário são: o app pede login de novo.
	for _, key := range append(legacyKeys, "owner", store.KeySessions) {
		if err := m.root.Delete(ctx, key); err != nil {
			return User{}, false, err
		}
	}
	m.log.Info("instalação de um usuário migrada para o espaço dele", "user", u.ID)
	return u, true, nil
}
