package ical

import (
	"bytes"
	"context"
	"fmt"

	"prazo/internal/httpx"
)

// Fetcher baixa e interpreta a URL de exportação do Moodle. A URL contém o
// authtoken do aluno: é credencial e nunca deve ser logada (use
// logging.RedactURL).
type Fetcher struct {
	URL    string
	Client *httpx.Client
}

// Fetch devolve os eventos e os erros de eventos individuais ignorados.
func (f *Fetcher) Fetch(ctx context.Context) ([]Event, []error, error) {
	body, err := f.Client.Get(ctx, f.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch ical: %w", err)
	}
	// O Moodle responde 200 com HTML quando o authtoken é inválido.
	if !bytes.Contains(body[:min(len(body), 512)], []byte("BEGIN:VCALENDAR")) {
		return nil, nil, fmt.Errorf("fetch ical: resposta não é iCalendar (authtoken inválido ou URL errada?)")
	}
	return Parse(bytes.NewReader(body))
}
