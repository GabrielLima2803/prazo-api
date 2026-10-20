package ical

import "errors"

var (
	ErrInvalidURL     = errors.New("ical: URL de exportação inválida")
	ErrNotICalendar   = errors.New("ical: resposta não é iCalendar (authtoken inválido ou URL errada?)")
	ErrMissingUID     = errors.New("evento sem UID")
	ErrMissingDTStart = errors.New("evento sem DTSTART")
)
