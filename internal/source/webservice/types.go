// Package webservice é o cliente da API REST do Moodle
// (webservice/rest/server.php) e dos endpoints públicos do app mobile.
package webservice

import (
	"fmt"

	"prazo/internal/collections"
	"prazo/internal/httpx"
)

const MobileService = "moodle_mobile_app"

// RequiredFunctions são as funções que o Prazo usa; o doctor confere se a
// instância libera cada uma.
var RequiredFunctions = []string{
	"core_webservice_get_site_info",
	"core_enrol_get_users_courses",
	"core_calendar_get_action_events_by_timesort",
	"core_calendar_get_calendar_upcoming_view",
	"mod_assign_get_assignments",
	"mod_assign_get_submission_status",
	"mod_quiz_get_quizzes_by_courses",
	"mod_quiz_get_user_attempts",
	"core_completion_get_activities_completion_status",
}

type Client struct {
	baseURL string
	http    *httpx.Client
}

type LoginType int

const (
	LoginApp             LoginType = 1
	LoginBrowser         LoginType = 2
	LoginEmbeddedBrowser LoginType = 3
)

func (t LoginType) String() string {
	switch t {
	case LoginApp:
		return "app (usuário e senha via /login/token.php)"
	case LoginBrowser:
		return "navegador (SSO)"
	case LoginEmbeddedBrowser:
		return "navegador embutido (SSO)"
	default:
		return fmt.Sprintf("desconhecido (%d)", int(t))
	}
}

type IdentityProvider struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// PublicConfig é o subconjunto útil de tool_mobile_get_public_config.
type PublicConfig struct {
	SiteName            string             `json:"sitename"`
	WWWRoot             string             `json:"wwwroot"`
	Lang                string             `json:"lang"`
	EnableWebServices   int                `json:"enablewebservices"`
	EnableMobileService int                `json:"enablemobilewebservice"`
	TypeOfLogin         LoginType          `json:"typeoflogin"`
	ShowLoginForm       int                `json:"showloginform"`
	MaintenanceEnabled  int                `json:"maintenanceenabled"`
	LaunchURL           string             `json:"launchurl"`
	IdentityProviders   []IdentityProvider `json:"identityproviders"`
}

type TokenProbe struct {
	Available bool
	ErrorCode string
	Message   string
}

type SiteFunction struct {
	Name string `json:"name"`
}

type SiteInfo struct {
	SiteName  string         `json:"sitename"`
	UserID    int64          `json:"userid"`
	Release   string         `json:"release"`
	Version   string         `json:"version"`
	Lang      string         `json:"lang"`
	Functions []SiteFunction `json:"functions"`
}

func (s SiteInfo) FunctionSet() collections.Set[string] {
	set := make(collections.Set[string], len(s.Functions))
	for _, f := range s.Functions {
		set.Add(f.Name)
	}
	return set
}

type Course struct {
	ID        int64  `json:"id"`
	ShortName string `json:"shortname"`
	FullName  string `json:"fullname"`
}

type EventCourse struct {
	ID        int64  `json:"id"`
	ShortName string `json:"shortname"`
}

// ActionEvent é um evento que ainda exige ação do aluno (não entregue).
type ActionEvent struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	ModuleName string      `json:"modulename"`
	EventType  string      `json:"eventtype"`
	TimeSort   int64       `json:"timesort"`
	URL        string      `json:"url"`
	Course     EventCourse `json:"course"`
}

type tokenResponse struct {
	Token     string `json:"token"`
	Error     string `json:"error"`
	ErrorCode string `json:"errorcode"`
}

type exceptionResponse struct {
	Exception string `json:"exception"`
	ErrorCode string `json:"errorcode"`
	Message   string `json:"message"`
}
