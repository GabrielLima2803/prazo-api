package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(retries int) *Client {
	c := New(Options{Timeout: 2 * time.Second, MaxRetries: retries})
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	body, err := newTestClient(3).Get(context.Background(), srv.URL)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("esperava 3 chamadas, teve %d", calls.Load())
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(3).Get(context.Background(), srv.URL)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("esperava StatusError 403, veio %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("4xx não deve ter retry; chamadas=%d", calls.Load())
	}
}

func TestBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()

	c := New(Options{MaxBody: 10})
	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("esperava erro de limite de tamanho")
	}
}

func TestNetworkErrorDoesNotLeakToken(t *testing.T) {
	c := newTestClient(0)
	_, err := c.Get(context.Background(), "http://127.0.0.1:1/x?authtoken=SEGREDO")
	if err == nil || strings.Contains(err.Error(), "SEGREDO") {
		t.Fatalf("erro deveria existir e não conter o token: %v", err)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	c := New(Options{BaseDelay: time.Second, MaxDelay: 5 * time.Second})
	for attempt := 1; attempt < 20; attempt++ {
		if d := c.backoff(attempt, nil); d <= 0 || d > 5*time.Second {
			t.Fatalf("tentativa %d: backoff fora do intervalo: %v", attempt, d)
		}
	}
}
