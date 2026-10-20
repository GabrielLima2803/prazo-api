// Package httpx é o cliente HTTP usado para falar com o Moodle: timeout por
// tentativa, retry com backoff exponencial + jitter, respeito a Retry-After,
// limite de tamanho de resposta e limite de requisições simultâneas.
package httpx

import (
	"context"
	"net/http"
	"time"
)

type Options struct {
	Timeout     time.Duration // por tentativa
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxBody     int64
	Concurrency int // padrão 1: não sobrecarregar o servidor da universidade
	UserAgent   string
	Transport   http.RoundTripper // nil = http.DefaultTransport
}

type Client struct {
	http       *http.Client
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	maxBody    int64
	userAgent  string
	sem        chan struct{}
	sleep      func(context.Context, time.Duration) error
}

type requestFunc func(context.Context) (*http.Request, error)
