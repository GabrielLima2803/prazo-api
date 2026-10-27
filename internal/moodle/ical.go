package moodle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
)

// ICalEvent é um VEVENT cru, sem regra de negócio aplicada.
type ICalEvent struct {
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
	Events  []ICalEvent
	Invalid []error
}

// ICalClient baixa a URL de exportação. A URL contém o authtoken do aluno.
type ICalClient struct {
	url  string
	http *httpx.Client
}

// ICalSource adapta o ICalClient à interface Source.
type ICalSource struct {
	client  *ICalClient
	baseURL string
	log     *slog.Logger
}

var (
	ErrInvalidURL     = errors.New("ical: URL de exportação inválida")
	ErrNotICalendar   = errors.New("ical: resposta não é iCalendar (Moodle em manutenção ou URL errada?)")
	ErrAuthRejected   = fmt.Errorf("%w: o Moodle recusou a URL do calendário (a senha mudou?)", ErrUnauthorized)
	ErrMissingUID     = errors.New("evento sem UID")
	ErrMissingDTStart = errors.New("evento sem DTSTART")
)

var invalidAuth = []byte("Invalid authentication")

func NewICalClient(exportURL string, http *httpx.Client) (*ICalClient, error) {
	u, err := url.Parse(exportURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, ErrInvalidURL
	}
	return &ICalClient{url: exportURL, http: http}, nil
}

// BaseURL devolve "https://host" do Moodle, sem o authtoken.
func (c *ICalClient) BaseURL() string {
	u, _ := url.Parse(c.url)
	return u.Scheme + "://" + u.Host
}

func (c *ICalClient) Fetch(ctx context.Context) (Feed, error) {
	body, err := c.http.Get(ctx, c.url)
	if err != nil {
		return Feed{}, fmt.Errorf("fetch ical: %w", err)
	}
	// O Moodle responde 200 nos dois casos: com o texto exato "Invalid
	// authentication" quando o authtoken não vale mais (senha trocada), e com
	// uma página HTML em manutenção ou erro. Só o primeiro exige novo login.
	if bytes.Equal(bytes.TrimSpace(body), invalidAuth) {
		return Feed{}, ErrAuthRejected
	}
	if !bytes.Contains(body[:min(len(body), 512)], []byte("BEGIN:VCALENDAR")) {
		return Feed{}, ErrNotICalendar
	}
	return ParseICal(bytes.NewReader(body))
}
