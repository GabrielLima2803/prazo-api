package google

import (
	"errors"
	"net/http"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"prazo/internal/calendar"
)

var (
	ErrNotAuthorized  = errors.New("o Google ainda não foi autorizado; rode \"prazo auth\"")
	ErrNoRefreshToken = errors.New("o Google não devolveu um refresh token; remova o acesso do Prazo em https://myaccount.google.com/permissions e rode \"prazo auth\" de novo")
	ErrScopeDenied    = errors.New("a permissão de agenda não foi concedida; rode \"prazo auth\" e marque a caixa de acesso às agendas")
	ErrStateMismatch  = errors.New("resposta de autorização com state inválido")
)

func statusCode(err error) int {
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		return ge.Code
	}
	return 0
}

func hasReason(err error, reasons ...string) bool {
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		return false
	}
	for _, item := range ge.Errors {
		for _, r := range reasons {
			if item.Reason == r {
				return true
			}
		}
	}
	return false
}

// retryable segue a orientação do Google: 429, 5xx e 403 por cota.
func retryable(err error) bool {
	code := statusCode(err)
	return code == http.StatusTooManyRequests || code >= 500 ||
		(code == http.StatusForbidden && hasReason(err, "rateLimitExceeded", "userRateLimitExceeded"))
}

// classify converte erros de autenticação em calendar.ErrAuth, para que o
// sync pare em vez de falhar evento por evento.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var re *oauth2.RetrieveError
	if statusCode(err) == http.StatusUnauthorized || (errors.As(err, &re) && re.ErrorCode == "invalid_grant") {
		return errors.Join(calendar.ErrAuth, err)
	}
	return err
}
