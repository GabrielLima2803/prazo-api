// Package ical lê a exportação iCalendar do Moodle
// (calendar/export_execute.php).
package ical

import (
	"log/slog"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
)

// Event é um VEVENT cru, sem regra de negócio aplicada.
type Event struct {
	UID          string
	Summary      string
	Description  string
	Categories   []string
	Location     string
	URL          string
	Start        time.Time // UTC
	End          time.Time // UTC; igual a Start quando não há DTEND
	AllDay       bool
	LastModified time.Time
	Props        []string // nomes das propriedades presentes, ordenados
}

// Feed é o resultado de ler um calendário. Eventos malformados vão para
// Invalid em vez de abortar a leitura inteira.
type Feed struct {
	Events  []Event
	Invalid []error
}

// Client baixa a URL de exportação. A URL contém o authtoken do aluno.
type Client struct {
	url  string
	http *httpx.Client
}

// Source adapta o Client à interface source.Source.
type Source struct {
	client  *Client
	baseURL string
	log     *slog.Logger
}
