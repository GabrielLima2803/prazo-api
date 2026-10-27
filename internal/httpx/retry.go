package httpx

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/retry"
)

// backoff usa full jitter; um Retry-After do servidor tem precedência.
func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var se *StatusError
	if errors.As(lastErr, &se) && se.RetryAfter > 0 {
		return min(se.RetryAfter, c.maxDelay)
	}
	return retry.Backoff(attempt, c.baseDelay, c.maxDelay)
}

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Temporary()
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, ErrBodyTooLarge) &&
		!errors.Is(err, ErrInsecureRedirect) && !errors.Is(err, ErrTooManyRedirects)
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
