package tenant

import (
	"github.com/GabrielLima2803/prazo-sync/internal/storage"

	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type memKV map[string][]byte

func (m memKV) Get(_ context.Context, k string) ([]byte, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return nil, storage.ErrNotFound
}
func (m memKV) Put(_ context.Context, k string, v []byte) error { m[k] = v; return nil }
func (m memKV) Delete(_ context.Context, k string) error        { delete(m, k); return nil }

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func newManager(root memKV, max int) *Manager {
	return NewManager(root, func(u User, kv storage.KV) *Tenant { return &Tenant{User: u, KV: kv} }, discard, max)
}

func TestRegisterAndLimit(t *testing.T) {
	ctx := context.Background()
	m := newManager(memKV{}, 2)
	ana, err := m.Register(ctx, User{ID: IDFor("moodle.x", 1), Host: "moodle.x", MoodleUserID: 1, Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := m.Register(ctx, User{ID: ana.User.ID, Name: "Ana Souza"})
	if again != ana || again.User.Name != "Ana Souza" || again.User.CreatedAt.IsZero() {
		t.Fatalf("registrar de novo = %+v", again.User)
	}
	m.Register(ctx, User{ID: "b"})
	if _, err := m.Register(ctx, User{ID: "c"}); !errors.Is(err, ErrTooManyUsers) {
		t.Fatalf("limite: %v", err)
	}
	if _, err := m.Get(ctx, "nao-existe"); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("Get: %v", err)
	}
	if IDFor("Moodle.X", 1) != IDFor("moodle.x", 1) || IDFor("moodle.x", 1) == IDFor("moodle.x", 2) {
		t.Fatal("IDFor não é estável ou colide")
	}
}

func TestMigrateLegacy(t *testing.T) {
	ctx := context.Background()
	root := memKV{
		storage.KeyGoogleToken:    []byte("token"),
		storage.KeyGoogleCalendar: []byte("cal@group"),
		storage.KeyPreferences:    []byte(`{}`),
		storage.KeySessions:       []byte(`[]`),
		"owner":                   []byte(`{}`),
	}
	m := newManager(root, 0)
	env := "https://moodle.joinville.udesc.br/calendar/export_execute.php?userid=12345&authtoken=x"
	u, ok, err := m.MigrateLegacy(ctx, env)
	if err != nil || !ok || u.MoodleUserID != 12345 || u.Host != "moodle.joinville.udesc.br" {
		t.Fatalf("migração = %+v, %v, %v", u, ok, err)
	}
	ns := m.Namespace(u.ID)
	if b, _ := ns.Get(ctx, storage.KeyGoogleCalendar); string(b) != "cal@group" {
		t.Fatal("o id da agenda não foi para o espaço do usuário")
	}
	if _, err := ns.Get(ctx, storage.KeyMoodle); err != nil {
		t.Fatal("a conta do Moodle deveria ter sido criada a partir da PRAZO_ICAL_URL")
	}
	for _, k := range []string{storage.KeyGoogleToken, storage.KeySessions, "owner"} {
		if _, ok := root[k]; ok {
			t.Errorf("%s continuou no espaço global", k)
		}
	}
	if _, ok, _ := m.MigrateLegacy(ctx, env); ok {
		t.Fatal("a migração deveria rodar uma vez só")
	}
}

func TestJitterBounds(t *testing.T) {
	for range 200 {
		if d := jitter(30 * time.Minute); d < 27*time.Minute || d > 33*time.Minute {
			t.Fatalf("jitter = %v", d)
		}
	}
}

func TestSleepUntil(t *testing.T) {
	if !sleepUntil(context.Background(), time.Now().Add(30*time.Millisecond), 5*time.Millisecond) {
		t.Fatal("deveria terminar quando o horário passa")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepUntil(ctx, time.Now().Add(time.Hour), time.Millisecond) {
		t.Fatal("deveria parar com o contexto cancelado")
	}
}
