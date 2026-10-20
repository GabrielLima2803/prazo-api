package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"prazo/internal/logging"
)

var ErrBodyTooLarge = errors.New("resposta maior que o limite")

// StatusError é uma resposta não-2xx.
type StatusError struct {
	Code       int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string { return fmt.Sprintf("http status %d", e.Code) }

func (e *StatusError) Temporary() bool {
	return e.Code == http.StatusTooManyRequests || e.Code == http.StatusRequestTimeout || e.Code >= 500
}

// sanitize remove segredos da URL que o net/http embute nas mensagens de erro.
func sanitize(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = logging.RedactURL(ue.URL)
	}
	return err
}
