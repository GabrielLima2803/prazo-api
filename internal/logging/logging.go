// Package logging monta o logger slog do Prazo e garante que nenhum segredo
// (authtoken do iCal, wstoken do Moodle, tokens OAuth, senhas) chegue ao log.
package logging

import (
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

const redacted = "REDACTED"

// sensitiveParams são nomes de parâmetros de query/form cujo valor é segredo.
var sensitiveParams = map[string]struct{}{
	"authtoken":     {},
	"wstoken":       {},
	"token":         {},
	"privatetoken":  {},
	"password":      {},
	"passport":      {},
	"access_token":  {},
	"refresh_token": {},
	"client_secret": {},
	"code":          {},
}

// sensitiveInText pega "param=valor" em qualquer texto livre (mensagens de
// erro do net/http incluem a URL inteira, por exemplo).
var sensitiveInText = regexp.MustCompile(
	`(?i)\b(authtoken|wstoken|privatetoken|token|password|passport|access_token|refresh_token|client_secret)=([^&\s"'<>]+)`)

// sensitiveKeys são chaves de atributo slog cujo valor nunca deve ser logado.
var sensitiveKeys = []string{"token", "secret", "password", "authorization", "key"}

// RedactURL devolve a URL com o valor dos parâmetros sensíveis trocado por
// REDACTED. Se não for uma URL válida, aplica a redação textual.
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
		if _, ok := sensitiveParams[strings.ToLower(k)]; ok {
			q.Set(k, redacted)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// Redact remove segredos no formato "param=valor" de um texto livre.
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

// replaceAttr é o redator aplicado a todo atributo antes de ser escrito.
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

// New cria um logger com redação. format "json" (produção) ou "text" (dev).
func New(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
