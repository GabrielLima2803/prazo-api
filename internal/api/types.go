// Package api expõe o Prazo para o app móvel: prazos, disciplinas, status da
// sincronização e preferências. JSON sobre HTTP, autenticado por token.
package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/account"
	"github.com/GabrielLima2803/prazo-sync/internal/domain"
	"github.com/GabrielLima2803/prazo-sync/internal/prefs"
	"github.com/GabrielLima2803/prazo-sync/internal/sync"
)

type Deps struct {
	Runner       *sync.Runner
	Accounts     *account.Service
	Prefs        *prefs.Store
	Token        string
	CalendarName string
	// GoogleReady diz se já existe autorização do Google guardada.
	GoogleReady func(ctx context.Context) bool
	// GoogleAuth é nil quando a autorização pelo app não foi configurada
	// (falta PRAZO_PUBLIC_URL ou a credencial web).
	GoogleAuth  GoogleAuth
	Log         *slog.Logger
	PlanMaxAge  time.Duration
	SyncTimeout time.Duration
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
	Start(returnTo string) (authURL string, err error)
	Finish(ctx context.Context, state, code string) (returnTo string, err error)
	Disconnect(ctx context.Context) error
}

type googleAuthRequest struct {
	ReturnTo string `json:"returnTo"`
}

type googleAuthResponse struct {
	AuthURL string `json:"authUrl"`
}

type Run struct {
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	Total      int       `json:"total"`
	Created    int       `json:"created"`
	Updated    int       `json:"updated"`
	Hidden     int       `json:"hidden"`
	Failed     int       `json:"failed"`
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
	Connected   bool           `json:"connected"`
	Method      account.Method `json:"method,omitempty"`
	BaseURL     string         `json:"baseUrl,omitempty"`
	FullName    string         `json:"fullName,omitempty"`
	SiteName    string         `json:"siteName,omitempty"`
	ConnectedAt *time.Time     `json:"connectedAt,omitempty"`
	Events      int            `json:"events,omitempty"` // só na resposta de conexão
}

type moodleLogin struct {
	BaseURL  string `json:"baseUrl"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type moodleICal struct {
	ICalURL string `json:"icalUrl"`
}

// errorBody leva um código estável para o app decidir o que mostrar, além
// da mensagem em português.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}
