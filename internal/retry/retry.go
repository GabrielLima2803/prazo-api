// Package retry tem as primitivas de espera usadas pelos clientes HTTP
// (Moodle e Google): backoff exponencial com full jitter e sleep cancelável.
package retry

import (
	"context"
	"math/rand/v2"
	"time"
)

// Backoff devolve um valor aleatório em (0, min(base·2^(attempt-1), max)].
// attempt começa em 1.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	ceil := base << (attempt - 1)
	if ceil <= 0 || ceil > max {
		ceil = max
	}
	return time.Duration(rand.Int64N(int64(ceil)) + 1)
}

// Sleep espera d ou até ctx ser cancelado.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
