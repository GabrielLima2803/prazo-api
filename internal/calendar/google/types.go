// Package google implementa calendar.Sink com a Google Calendar API e o
// fluxo OAuth de app instalado (loopback + PKCE).
package google

import (
	"context"
	"time"

	gcal "google.golang.org/api/calendar/v3"
)

// Scope é o mínimo necessário: só agendas criadas pelo próprio Prazo. A
// agenda principal do usuário fica fora de alcance.
const Scope = gcal.CalendarAppCreatedScope

type Sink struct {
	svc        *gcal.Service
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	sleep      func(context.Context, time.Duration) error
}
