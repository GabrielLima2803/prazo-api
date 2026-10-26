package webservice

import (
	"errors"
	"fmt"
)

var (
	ErrServiceUnavailable = errors.New("web services do Moodle indisponíveis para esta instância")
	ErrInvalidLogin       = errors.New("usuário ou senha inválidos")
	// ErrAuthBackendDown: o Moodle não alcança o servidor de senhas (LDAP da
	// UDESC). Não é a senha do usuário; tentar mais tarde resolve.
	ErrAuthBackendDown    = errors.New("o login do Moodle está fora do ar (servidor de senhas da universidade)")
	ErrUnexpectedResponse = errors.New("resposta inesperada do Moodle")
)

// MoodleError é uma exceção devolvida pela API (que responde HTTP 200 mesmo em erro).
type MoodleError struct {
	Function string
	Code     string
	Message  string
}

func (e *MoodleError) Error() string {
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
