package calendar

import "errors"

var (
	// ErrAuth indica credencial revogada ou expirada: nenhuma escrita vai
	// funcionar até um novo "prazo auth".
	ErrAuth = errors.New("autorização do Google inválida; rode \"prazo auth\" de novo")
	// ErrCalendarGone indica que a agenda foi apagada pelo usuário.
	ErrCalendarGone = errors.New("agenda não existe mais")
)
