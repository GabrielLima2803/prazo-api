package ical

import (
	"bytes"
	"context"
	"fmt"
	"net/url"

	"github.com/GabrielLima2803/prazo-sync/internal/httpx"
)

var invalidAuth = []byte("Invalid authentication")

func NewClient(exportURL string, http *httpx.Client) (*Client, error) {
	u, err := url.Parse(exportURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, ErrInvalidURL
	}
	return &Client{url: exportURL, http: http}, nil
}

// BaseURL devolve "https://host" do Moodle, sem o authtoken.
func (c *Client) BaseURL() string {
	u, _ := url.Parse(c.url)
	return u.Scheme + "://" + u.Host
}

func (c *Client) Fetch(ctx context.Context) (Feed, error) {
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
	return Parse(bytes.NewReader(body))
}
