// Package api expõe o Prazo para o app móvel: prazos, disciplinas, status da
// sincronização e preferências. JSON sobre HTTP, autenticado por token.
package api

import (
	"github.com/GabrielLima2803/prazo-sync/internal/moodle"
	"github.com/GabrielLima2803/prazo-sync/internal/storage"
	"github.com/GabrielLima2803/prazo-sync/internal/tenant"

	"context"
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/domain"
)

type Deps struct {
	Tenants  *tenant.Manager
	Auth     *moodle.Authenticator
	Sessions *tenant.Sessions
	// Token é o PRAZO_API_TOKEN (terminal e administração); age como
	// AdminTenant.
	Token       string
	AdminTenant func(ctx context.Context) (*tenant.Tenant, error)
	// GoogleReady diz se o usuário já tem autorização do Google guardada.
	GoogleReady func(ctx context.Context, kv storage.KV) bool
	// GoogleAuth é nil quando a autorização pelo app não foi configurada
	// (falta PRAZO_PUBLIC_URL ou a credencial web).
	GoogleAuth GoogleAuth
	// ForgetGoogle revoga no Google e apaga o token do usuário (excluir conta).
	// Pode devolver calendar.ErrRevoke quando só a parte local deu certo.
	ForgetGoogle func(ctx context.Context, kv storage.KV) error
	// CORSOrigins são as origens liberadas para o app no navegador; vazio
	// desliga o CORS (o app nativo não precisa).
	CORSOrigins []string
	// Privacy identifica quem responde pelos dados na página /privacidade.
	Privacy      Privacy
	CalendarName string
	Log          *slog.Logger
	PlanMaxAge   time.Duration
	SyncTimeout  time.Duration
}

// Privacy são os dados do controlador exigidos pela LGPD (art. 9º e 41).
type Privacy struct {
	Operator string // quem mantém o servidor
	Contact  string // e-mail para pedidos do titular
}

type Deadline struct {
	ID           string              `json:"id"`
	CourseCode   string              `json:"courseCode"`
	CourseName   string              `json:"courseName"`
	Type         domain.DeadlineType `json:"type"`
	Label        string              `json:"label"`
	Phase        domain.Phase        `json:"phase"`
	Title        string              `json:"title"`
	DisplayTitle string              `json:"displayTitle"`
	Description  string              `json:"description"`
	URL          string              `json:"url"`
	StartsAt     time.Time           `json:"startsAt"`
	EndsAt       time.Time           `json:"endsAt"`
	Instant      bool                `json:"instant"`
	Submitted    bool                `json:"submitted"`
	ColorID      string              `json:"colorId"`
	Color        string              `json:"color"`
	Reminders    []int               `json:"reminders"` // minutos antes
	Hidden       bool                `json:"hidden"`
}

type DeadlineList struct {
	Deadlines []Deadline `json:"deadlines"`
	FetchedAt time.Time  `json:"fetchedAt"`
}

type Course struct {
	Code    string     `json:"code"`
	Name    string     `json:"name"`
	ColorID string     `json:"colorId"`
	Color   string     `json:"color"`
	Hidden  bool       `json:"hidden"`
	Total   int        `json:"total"`
	Pending int        `json:"pending"` // prazos ainda no futuro
	NextDue *time.Time `json:"nextDue"`
}

type Status struct {
	Now     time.Time  `json:"now"`
	Google  Google     `json:"google"`
	LastRun *Run       `json:"lastRun"`
	NextRun *time.Time `json:"nextRun"`
}

type Google struct {
	Authorized   bool   `json:"authorized"`
	CalendarName string `json:"calendarName"`
	// AppAuth diz se o app pode iniciar a autorização (botão "Conectar Google").
	AppAuth bool `json:"appAuth"`
}

// GoogleAuth inicia e conclui a autorização do Google começada no app.
type GoogleAuth interface {
	Start(userID, returnTo string) (authURL string, err error)
	Finish(ctx context.Context, state, code string) (userID, returnTo string, err error)
	Disconnect(ctx context.Context, userID string) error
}

type GoogleAuthRequest struct {
	ReturnTo string `json:"returnTo"`
}

type GoogleAuthResponse struct {
	AuthURL string `json:"authUrl"`
}

type Run struct {
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
	Total         int       `json:"total"`
	Created       int       `json:"created"`
	Updated       int       `json:"updated"`
	Unchanged     int       `json:"unchanged"`
	Deleted       int       `json:"deleted"`
	DeleteBlocked int       `json:"deleteBlocked"`
	Hidden        int       `json:"hidden"`
	Failed        int       `json:"failed"`
}

type Meta struct {
	Palette []Color `json:"palette"`
	Types   []Type  `json:"types"`
}

type Color struct {
	ID  string `json:"id"`
	Hex string `json:"hex"`
}

type Type struct {
	ID               domain.DeadlineType `json:"id"`
	Label            string              `json:"label"`
	DefaultReminders []int               `json:"defaultReminders"`
}

type MoodleStatus struct {
	Connected   bool          `json:"connected"`
	Method      moodle.Method `json:"method,omitempty"`
	BaseURL     string        `json:"baseUrl,omitempty"`
	FullName    string        `json:"fullName,omitempty"`
	SiteName    string        `json:"siteName,omitempty"`
	ConnectedAt *time.Time    `json:"connectedAt,omitempty"`
	NeedsReauth bool          `json:"needsReauth,omitempty"`
	Events      int           `json:"events,omitempty"` // só na resposta de conexão
}

type MoodleLogin struct {
	BaseURL  string `json:"baseUrl"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token  string       `json:"token"`
	Moodle MoodleStatus `json:"moodle"`
}

type MoodleICal struct {
	ICalURL string `json:"icalUrl"`
}

// ErrorBody leva um código estável para o app decidir o que mostrar, além
// da mensagem em português.
type ErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}
