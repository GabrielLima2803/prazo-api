package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackoffBounded(t *testing.T) {
	for attempt := 1; attempt <= 70; attempt++ { // inclui shifts que estouram int64
		if d := Backoff(attempt, time.Second, 30*time.Second); d <= 0 || d > 30*time.Second {
			t.Fatalf("tentativa %d: %v fora de (0, 30s]", attempt, d)
		}
	}
	for range 100 {
		if d := Backoff(1, time.Second, 30*time.Second); d > time.Second {
			t.Fatalf("primeira tentativa: %v > 1s", d)
		}
	}
}

func TestSleepCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
