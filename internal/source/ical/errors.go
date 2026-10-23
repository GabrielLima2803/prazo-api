package ical

import (
	"errors"
	"fmt"

	"github.com/GabrielLima2803/prazo-sync/internal/source"
)

var (
	ErrInvalidURL     = errors.New("ical: URL de exportação inválida")
	ErrNotICalendar   = errors.New("ical: resposta não é iCalendar (Moodle em manutenção ou URL errada?)")
	ErrAuthRejected   = fmt.Errorf("%w: o Moodle recusou a URL do calendário (a senha mudou?)", source.ErrUnauthorized)
	ErrMissingUID     = errors.New("evento sem UID")
	ErrMissingDTStart = errors.New("evento sem DTSTART")
)
