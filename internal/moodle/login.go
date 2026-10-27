package moodle

import (
	"github.com/GabrielLima2803/prazo-sync/internal/httpx"

	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/GabrielLima2803/prazo-sync/internal/collections"
)

// NewAuthenticator restringe o login aos hosts de allowedHosts, para que o
// servidor não faça requisições a qualquer endereço informado no app (SSRF).
// Sem nenhum host, nenhum login é aceito.
func NewAuthenticator(http *httpx.Client, allowedHosts ...string) *Authenticator {
	set := collections.Set[string]{}
	for _, h := range allowedHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			set.Add(h)
		}
	}
	return &Authenticator{http: http, allowed: set}
}

// Login troca usuário e senha por um token de web service, pede ao Moodle o
// authtoken da exportação e monta a URL do iCal. A senha só é usada aqui.
func (a *Authenticator) Login(ctx context.Context, baseURL, username, password string) (Account, error) {
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return Account{}, err
	}
	if err := a.checkHost(base); err != nil {
		return Account{}, err
	}
	ws := NewWebServiceClient(base, a.http)
	token, err := ws.Login(ctx, strings.TrimSpace(username), password)
	if err != nil {
		return Account{}, err
	}
	info, err := ws.SiteInfo(ctx, token)
	if err != nil {
		return Account{}, fmt.Errorf("dados do usuário: %w", err)
	}
	if !info.FunctionSet().Has("core_calendar_get_calendar_export_token") {
		return Account{}, ErrExportUnavailable
	}
	exportToken, err := ws.CalendarExportToken(ctx, token)
	if err != nil {
		return Account{}, fmt.Errorf("%w (%w)", ErrExportUnavailable, err)
	}
	return Account{
		Method:   MethodLogin,
		BaseURL:  base,
		ICalURL:  exportURL(base, info.UserID, exportToken),
		Token:    token,
		UserID:   info.UserID,
		FullName: info.FullName,
		SiteName: info.SiteName,
	}, nil
}

// FromICal identifica a pessoa por uma URL de exportação colada no app. Quem
// tem uma URL válida (com authtoken) é dono daquele calendário; Service.Save
// confere baixando o calendário antes de guardar.
func (a *Authenticator) FromICal(icalURL string) (Account, error) {
	icalURL = strings.TrimSpace(icalURL)
	c, err := NewICalClient(icalURL, a.http)
	if err != nil {
		return Account{}, err
	}
	if err := a.checkHost(c.BaseURL()); err != nil {
		return Account{}, err
	}
	userID := userIDOf(icalURL)
	if userID == 0 {
		return Account{}, ErrMissingUserID
	}
	return Account{Method: MethodICal, BaseURL: c.BaseURL(), ICalURL: icalURL, UserID: userID}, nil
}

func (a *Authenticator) checkHost(base string) error {
	if !a.allowed.Has(hostOf(base)) {
		return ErrHostNotAllowed
	}
	return nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", ErrInvalidBaseURL
	}
	return "https://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

// exportURL reproduz a URL de "Exportar calendário → Todos os eventos".
func exportURL(base string, userID int64, authToken string) string {
	q := url.Values{
		"userid":      {strconv.FormatInt(userID, 10)},
		"authtoken":   {authToken},
		"preset_what": {"all"},
		"preset_time": {"custom"},
	}
	return base + "/calendar/export_execute.php?" + q.Encode()
}

func userIDOf(raw string) int64 {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(u.Query().Get("userid"), 10, 64)
	return id
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// IdentityFromICal devolve host e userid de uma URL de exportação, usada para
// migrar a instalação de um usuário só para o modelo multiusuário.
func IdentityFromICal(raw string) (host string, userID int64) {
	return hostOf(raw), userIDOf(raw)
}
