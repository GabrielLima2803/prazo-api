package moodle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
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

type WebServiceClient struct {
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
	FullName  string         `json:"fullname"`
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

var (
	ErrServiceUnavailable = errors.New("web services do Moodle indisponíveis para esta instância")
	ErrInvalidLogin       = errors.New("usuário ou senha inválidos")
	// ErrAuthBackendDown: o Moodle não alcança o servidor de senhas (LDAP da
	// UDESC). Não é a senha do usuário; tentar mais tarde resolve.
	ErrAuthBackendDown    = errors.New("o login do Moodle está fora do ar (servidor de senhas da universidade)")
	ErrUnexpectedResponse = errors.New("resposta inesperada do Moodle")
)

// WebServiceError é uma exceção devolvida pela API (que responde HTTP 200 mesmo em erro).
type WebServiceError struct {
	Function string
	Code     string
	Message  string
}

func (e *WebServiceError) Error() string {
	return fmt.Sprintf("%s: moodle: %s (%s)", e.Function, e.Message, e.Code)
}

func loginError(code, message string) error {
	switch code {
	case "invalidlogin":
		return ErrInvalidLogin
	case "servicenotavailable", "enablewsdescription":
		return fmt.Errorf("%w (%s)", ErrServiceUnavailable, code)
	case "auth_ldap_noconnect", "auth_ldap_noconnect_all":
		return fmt.Errorf("%w (%s)", ErrAuthBackendDown, code)
	default:
		return fmt.Errorf("%s (%s)", message, code)
	}
}

func NewWebServiceClient(baseURL string, http *httpx.Client) *WebServiceClient {
	return &WebServiceClient{baseURL: strings.TrimRight(baseURL, "/"), http: http}
}

func (c *WebServiceClient) BaseURL() string { return c.baseURL }

// Call chama uma função REST e decodifica a resposta em out. O token vai no
// corpo do POST, nunca na URL.
func (c *WebServiceClient) Call(ctx context.Context, token, function string, args url.Values, out any) error {
	form := url.Values{"wstoken": {token}, "wsfunction": {function}, "moodlewsrestformat": {"json"}}
	maps.Copy(form, args)

	body, err := c.http.PostForm(ctx, c.baseURL+"/webservice/rest/server.php", form)
	if err != nil {
		return fmt.Errorf("%s: %w", function, err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		var e exceptionResponse
		if json.Unmarshal(body, &e) == nil && e.Exception != "" {
			return &WebServiceError{Function: function, Code: e.ErrorCode, Message: e.Message}
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: %w: %v", function, ErrUnexpectedResponse, err)
	}
	return nil
}

// callAJAX chama um endpoint público (sem login) de lib/ajax/service-nologin.php.
func (c *WebServiceClient) callAJAX(ctx context.Context, method string, out any) error {
	req, err := json.Marshal([]map[string]any{{"index": 0, "methodname": method, "args": map[string]any{}}})
	if err != nil {
		return err
	}
	body, err := c.http.PostJSON(ctx, c.baseURL+"/lib/ajax/service-nologin.php?info="+url.QueryEscape(method), req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	var resp []struct {
		Error bool            `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp) != 1 || resp[0].Error {
		return fmt.Errorf("%s: %w", method, ErrUnexpectedResponse)
	}
	if err := json.Unmarshal(resp[0].Data, out); err != nil {
		return fmt.Errorf("%s: %w: %v", method, ErrUnexpectedResponse, err)
	}
	return nil
}
