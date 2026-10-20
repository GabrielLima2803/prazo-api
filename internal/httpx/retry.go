package httpx

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// backoff usa "full jitter": aleatório entre 0 e min(base·2^(n-1), max).
// Um Retry-After do servidor tem precedência.
func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var se *StatusError
	if errors.As(lastErr, &se) && se.RetryAfter > 0 {
		return min(se.RetryAfter, c.maxDelay)
	}
	ceil := c.baseDelay << (attempt - 1)
	if ceil <= 0 || ceil > c.maxDelay {
		ceil = c.maxDelay
	}
	return time.Duration(rand.Int64N(int64(ceil)) + 1)
}

func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Temporary()
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, ErrBodyTooLarge)
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
