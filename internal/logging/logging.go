// Package logging cria o logger slog do Prazo com redação de segredos
// (authtoken, wstoken, tokens OAuth, senhas).
package logging

import (
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"prazo/internal/collections"
)

const redacted = "REDACTED"

var sensitiveParams = collections.NewSet(
	"authtoken", "wstoken", "token", "privatetoken", "password", "passport",
	"access_token", "refresh_token", "client_secret", "code",
)

// Mensagens de erro do net/http incluem a URL inteira.
var sensitiveInText = regexp.MustCompile(
	`(?i)\b(authtoken|wstoken|privatetoken|token|password|passport|access_token|refresh_token|client_secret)=([^&\s"'<>]+)`)

var sensitiveKeys = []string{"token", "secret", "password", "authorization", "key"}

// RedactURL troca o valor dos parâmetros sensíveis por REDACTED.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return Redact(raw)
	}
	if u.User != nil {
		u.User = url.User(redacted)
	}
	q := u.Query()
	changed := false
	for k := range q {
		if sensitiveParams.Has(strings.ToLower(k)) {
			q.Set(k, redacted)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// Redact remove segredos "param=valor" de um texto livre.
func Redact(s string) string {
	return sensitiveInText.ReplaceAllString(s, "${1}="+redacted)
}

func isSensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, redacted)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Redact(a.Value.String()))
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Redact(err.Error()))
		}
	}
	return a
}

// New cria o logger; format é "json" (produção) ou "text" (dev).
func New(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
