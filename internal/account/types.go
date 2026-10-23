// Package account guarda as contas conectadas pelo app. Hoje, a do Moodle:
// a URL do iCal e o token de web service, cifrados no KV. A senha nunca é
// guardada.
package account

import (
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

type Method string

const (
	MethodLogin Method = "login" // usuário e senha → token → URL do iCal montada pelo Prazo
	MethodICal  Method = "ical"  // URL colada pelo usuário (contas só com SSO)
	MethodEnv   Method = "env"   // PRAZO_ICAL_URL, sem conta salva
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

// Owner é a conta do Moodle dona do servidor. Enquanto o Prazo for de um
// usuário só, apenas ela consegue entrar pelo app.
type Owner struct {
	Host   string `json:"host"`
	UserID int64  `json:"userId"`
}

type Service struct {
	kv      store.KV // nil: só a variável de ambiente
	http    *httpx.Client
	log     *slog.Logger
	envICal string
	now     func() time.Time
}
