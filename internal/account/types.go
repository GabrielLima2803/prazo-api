// Package account cuida da conta do Moodle de cada usuário: quem é a pessoa
// (Authenticator) e o que fica guardado no espaço dela (Service). A senha
// nunca é guardada; a URL do iCal e o token ficam cifrados no KV do usuário.
package account

import (
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type Method string

const (
	MethodLogin Method = "login" // usuário e senha → token → URL do iCal montada pelo Prazo
	MethodICal  Method = "ical"  // URL colada pelo usuário (contas só com SSO)
	MethodEnv   Method = "env"   // PRAZO_ICAL_URL, sem conta salva (uso pelo terminal)
)

// Moodle é a conta salva. ICalURL e Token são segredos e nunca saem pela API.
type Moodle struct {
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
func (m Moodle) Host() string { return hostOf(m.BaseURL) }

// Authenticator descobre quem é a pessoa no Moodle, sem guardar nada.
type Authenticator struct {
	http    *httpx.Client
	allowed collections.Set[string] // hosts aceitos; vazio = qualquer um
}

// Service é a conta de um usuário, no KV dele.
type Service struct {
	kv      store.KV // nil: só a variável de ambiente
	http    *httpx.Client
	log     *slog.Logger
	envICal string
	now     func() time.Time
}
