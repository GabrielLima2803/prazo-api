package account

import "errors"

var (
	ErrNotConnected      = errors.New("nenhuma conta do Moodle conectada")
	ErrInvalidBaseURL    = errors.New("o endereço do Moodle precisa começar com https://")
	ErrExportUnavailable = errors.New("este Moodle não libera a exportação do calendário pelo app; cole a URL do iCal")
	ErrEmptyFeed         = errors.New("o calendário exportado veio vazio; confira se é a conta certa")
	ErrNotOwner          = errors.New("este servidor do Prazo pertence a outra conta do Moodle")
	ErrMissingUserID     = errors.New("a URL do calendário não tem o userid; gere de novo em Calendário → Exportar")
)
