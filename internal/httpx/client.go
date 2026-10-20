// Package httpx é um cliente HTTP "educado" para falar com o Moodle:
// timeout por tentativa, retry com backoff exponencial + jitter, respeito a
// Retry-After, limite de tamanho de resposta e no máximo N requisições
// simultâneas (padrão 1, para não pesar no servidor da universidade).
package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"prazo/internal/logging"
)

// StatusError é uma resposta HTTP não-2xx.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("http status %d", e.Code) }

// Retryable diz se vale a pena tentar de novo.
func (e *StatusError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code == http.StatusRequestTimeout || e.Code >= 500
}

// Client deve ser criado com New. É seguro para uso concorrente.
type Client struct {
	http       *http.Client
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	maxBody    int64
	userAgent  string
	sem        chan struct{} // semáforo: limita requisições simultâneas
	sleep      func(context.Context, time.Duration) error
}

type Options struct {
	Timeout     time.Duration // por tentativa
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxBody     int64
	Concurrency int
	UserAgent   string
}

func New(o Options) *Client {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
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
		o.UserAgent = "prazo/0.1 (+sincronizador Moodle -> Google Agenda)"
	}
	return &Client{
		http:       &http.Client{Timeout: o.Timeout},
		maxRetries: o.MaxRetries,
		baseDelay:  o.BaseDelay,
		maxDelay:   o.MaxDelay,
		maxBody:    o.MaxBody,
		userAgent:  o.UserAgent,
		sem:        make(chan struct{}, o.Concurrency),
		sleep:      sleepCtx,
	}
}

// Get baixa a URL e devolve o corpo.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	})
}

// PostForm envia um formulário. Use para mandar credenciais (no corpo, nunca
// na URL, para não acabarem em logs de proxy).
func (c *Client) PostForm(ctx context.Context, rawURL string, form url.Values) ([]byte, error) {
	body := form.Encode()
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
}

// PostJSON envia um corpo JSON já serializado.
func (c *Client) PostJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	return c.do(ctx, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

func (c *Client) do(ctx context.Context, newReq func(context.Context) (*http.Request, error)) ([]byte, error) {
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

func (c *Client) once(ctx context.Context, newReq func(context.Context) (*http.Request, error)) ([]byte, error) {
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
		return nil, &retryAfterError{StatusError: &StatusError{Code: resp.StatusCode}, after: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, sanitize(err)
	}
	if int64(len(body)) > c.maxBody {
		return nil, fmt.Errorf("resposta maior que o limite de %d bytes", c.maxBody)
	}
	return body, nil
}

// backoff: base * 2^(n-1), limitado a maxDelay, com "full jitter" (valor
// aleatório entre 0 e o teto). Se o servidor mandou Retry-After, ele manda.
func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var ra *retryAfterError
	if errors.As(lastErr, &ra) && ra.after > 0 {
		return min(ra.after, c.maxDelay)
	}
	ceil := c.baseDelay << (attempt - 1)
	if ceil <= 0 || ceil > c.maxDelay {
		ceil = c.maxDelay
	}
	return time.Duration(rand.Int64N(int64(ceil)) + 1)
}

type retryAfterError struct {
	*StatusError
	after time.Duration
}

func (e *retryAfterError) Unwrap() error { return e.StatusError }

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Retryable()
	}
	// Erros de rede/timeout: tenta de novo. Cancelamento do chamador, não.
	return !errors.Is(err, context.Canceled)
}

// sanitize tira segredos da URL que o net/http embute nas mensagens de erro.
func sanitize(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = logging.RedactURL(ue.URL)
	}
	return err
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
