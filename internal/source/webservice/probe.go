// Package webservice fala com a API REST do Moodle. Na Fase 0 só tem o que o
// doctor precisa para descobrir o que a instância permite.
package webservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"prazo/internal/httpx"
)

const MobileService = "moodle_mobile_app"

// Functions que o Prazo pretende usar nas fases seguintes.
var WantedFunctions = []string{
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
	BaseURL string // ex.: https://www.moodle.udesc.br
	HTTP    *httpx.Client
}

// PublicConfig é o subconjunto útil de tool_mobile_get_public_config, que o
// app oficial chama antes do login (não exige credencial).
type PublicConfig struct {
	SiteName            string `json:"sitename"`
	WWWRoot             string `json:"wwwroot"`
	EnableWebServices   int    `json:"enablewebservices"`
	EnableMobileService int    `json:"enablemobilewebservice"`
	TypeOfLogin         int    `json:"typeoflogin"` // 1 app, 2 browser SSO, 3 embedded browser
	Lang                string `json:"lang"`
	MaintenanceEnabled  int    `json:"maintenanceenabled"`
	ShowLoginForm       int    `json:"showloginform"`
	LaunchURL           string `json:"launchurl"`
	IdentityProviders   []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"identityproviders"`
}

func (p PublicConfig) LoginTypeName() string {
	switch p.TypeOfLogin {
	case 1:
		return "app (usuário e senha via /login/token.php)"
	case 2:
		return "navegador (SSO)"
	case 3:
		return "navegador embutido (SSO)"
	default:
		return fmt.Sprintf("desconhecido (%d)", p.TypeOfLogin)
	}
}

func (c *Client) PublicConfig(ctx context.Context) (PublicConfig, error) {
	req := `[{"index":0,"methodname":"tool_mobile_get_public_config","args":{}}]`
	body, err := c.HTTP.PostJSON(ctx, c.BaseURL+"/lib/ajax/service-nologin.php?info=tool_mobile_get_public_config", []byte(req))
	if err != nil {
		return PublicConfig{}, fmt.Errorf("public config: %w", err)
	}
	var resp []struct {
		Error     bool            `json:"error"`
		Data      PublicConfig    `json:"data"`
		Exception json.RawMessage `json:"exception"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp) != 1 {
		return PublicConfig{}, fmt.Errorf("public config: resposta inesperada")
	}
	if resp[0].Error {
		return PublicConfig{}, fmt.Errorf("public config: moodle retornou erro")
	}
	return resp[0].Data, nil
}

// TokenProbe é o diagnóstico de /login/token.php sem credenciais reais.
type TokenProbe struct {
	ErrorCode string
	Available bool // true se o serviço respondeu "login inválido" (ou seja, existe)
	Message   string
}

// ProbeTokenEndpoint tenta logar com um usuário aleatório inexistente. O código
// de erro diz se o serviço mobile está habilitado sem precisar de senha real:
//   - invalidlogin       → serviço ativo, login por senha aceito
//   - servicenotavailable / enablewsdescription → bloqueado
func (c *Client) ProbeTokenEndpoint(ctx context.Context) (TokenProbe, error) {
	form := url.Values{
		"username": {"prazo-doctor-" + randomHex(6)},
		"password": {randomHex(12)},
		"service":  {MobileService},
	}
	body, err := c.HTTP.PostForm(ctx, c.BaseURL+"/login/token.php", form)
	if err != nil {
		return TokenProbe{}, fmt.Errorf("probe token.php: %w", err)
	}
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return TokenProbe{}, fmt.Errorf("probe token.php: resposta não é JSON")
	}
	p := TokenProbe{ErrorCode: r.ErrorCode, Message: r.Error}
	p.Available = r.ErrorCode == "invalidlogin"
	return p, nil
}

type tokenResponse struct {
	Token     string `json:"token"`
	Error     string `json:"error"`
	ErrorCode string `json:"errorcode"`
}

// ErrServiceUnavailable indica que a instância não permite obter token.
var ErrServiceUnavailable = errors.New("serviço de web services do Moodle indisponível")

// Login troca usuário+senha por um token. A senha não é guardada em lugar nenhum.
func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	form := url.Values{"username": {username}, "password": {password}, "service": {MobileService}}
	body, err := c.HTTP.PostForm(ctx, c.BaseURL+"/login/token.php", form)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("login: resposta não é JSON")
	}
	switch {
	case r.Token != "":
		return r.Token, nil
	case r.ErrorCode == "servicenotavailable" || r.ErrorCode == "enablewsdescription":
		return "", fmt.Errorf("login: %w (%s)", ErrServiceUnavailable, r.ErrorCode)
	default:
		return "", fmt.Errorf("login: %s (%s)", r.Error, r.ErrorCode)
	}
}

// SiteInfo é o subconjunto útil de core_webservice_get_site_info.
type SiteInfo struct {
	SiteName  string `json:"sitename"`
	UserID    int64  `json:"userid"`
	Release   string `json:"release"`
	Version   string `json:"version"`
	Lang      string `json:"lang"`
	Functions []struct {
		Name string `json:"name"`
	} `json:"functions"`
}

// FunctionSet devolve as funções disponíveis como conjunto, para checagem O(1).
func (s SiteInfo) FunctionSet() map[string]struct{} {
	set := make(map[string]struct{}, len(s.Functions))
	for _, f := range s.Functions {
		set[f.Name] = struct{}{}
	}
	return set
}

func (c *Client) SiteInfo(ctx context.Context, token string) (SiteInfo, error) {
	var si SiteInfo
	if err := c.Call(ctx, token, "core_webservice_get_site_info", nil, &si); err != nil {
		return SiteInfo{}, err
	}
	return si, nil
}

// Call chama uma função REST. O token vai no corpo do POST, nunca na URL.
func (c *Client) Call(ctx context.Context, token, fn string, args url.Values, out any) error {
	form := url.Values{"wstoken": {token}, "wsfunction": {fn}, "moodlewsrestformat": {"json"}}
	for k, v := range args {
		form[k] = v
	}
	body, err := c.HTTP.PostForm(ctx, c.BaseURL+"/webservice/rest/server.php", form)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}
	// Erros do Moodle vêm com HTTP 200 e um objeto {exception, errorcode, message}.
	if trimmed := strings.TrimSpace(string(body)); strings.HasPrefix(trimmed, "{") {
		var e struct {
			Exception string `json:"exception"`
			ErrorCode string `json:"errorcode"`
			Message   string `json:"message"`
		}
		if json.Unmarshal(body, &e) == nil && e.Exception != "" {
			return fmt.Errorf("%s: moodle: %s (%s)", fn, e.Message, e.ErrorCode)
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decode: %w", fn, err)
	}
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
