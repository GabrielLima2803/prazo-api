package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/retry"
)

func New(o Options) *Client {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = time.Second
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = 30 * time.Second
	}
	if o.MaxBody <= 0 {
		o.MaxBody = 20 << 20
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 1
	}
	if o.UserAgent == "" {
		o.UserAgent = "github.com/GabrielLima2803/prazo-sync/0.1"
	}
	return &Client{
		http:       &http.Client{Timeout: o.Timeout, Transport: o.Transport, CheckRedirect: checkRedirect},
		maxRetries: max(o.MaxRetries, 0),
		baseDelay:  o.BaseDelay,
		maxDelay:   o.MaxDelay,
		maxBody:    o.MaxBody,
		userAgent:  o.UserAgent,
		sem:        make(chan struct{}, o.Concurrency),
		sleep:      retry.Sleep,
	}
}

// maxRedirects segue o padrão do net/http.
const maxRedirects = 10

// checkRedirect só segue redirecionamentos para HTTPS: um Moodle (ou quem se
// passe por ele) não consegue levar o servidor a um endereço http interno,
// nem fazer as credenciais trafegarem sem TLS.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return ErrTooManyRedirects
	}
	if req.URL.Scheme != "https" {
		return ErrInsecureRedirect
	}
	return nil
}

func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	})
}

// PostForm envia credenciais no corpo, nunca na URL, para não acabarem em
// logs de proxy.
func (c *Client) PostForm(ctx context.Context, rawURL string, form url.Values) ([]byte, error) {
	body := form.Encode()
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return req, err
	})
}

func (c *Client) PostJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, err
	})
}

func (c *Client) do(ctx context.Context, newReq requestFunc) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, c.backoff(attempt, lastErr)); err != nil {
				return nil, errors.Join(err, lastErr)
			}
		}
		body, err := c.once(ctx, newReq)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable(err) || ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, newReq requestFunc) ([]byte, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.sem }()

	req, err := newReq(ctx)
	if err != nil {
		return nil, sanitize(err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, sanitize(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, &StatusError{Code: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, sanitize(err)
	}
	if int64(len(body)) > c.maxBody {
		return nil, fmt.Errorf("%w (%d bytes)", ErrBodyTooLarge, c.maxBody)
	}
	return body, nil
}
