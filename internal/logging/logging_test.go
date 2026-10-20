package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

const secret = "s3cr3t-abc123"

func TestRedactURL(t *testing.T) {
	got := RedactURL("https://www.moodle.udesc.br/calendar/export_execute.php?userid=42&authtoken=" + secret + "&preset_what=all")
	if strings.Contains(got, secret) {
		t.Fatalf("segredo vazou: %s", got)
	}
	if !strings.Contains(got, "userid=42") || !strings.Contains(got, "preset_what=all") {
		t.Fatalf("parâmetros não sensíveis deveriam ser mantidos: %s", got)
	}
}

func TestRedactText(t *testing.T) {
	msg := `Get "https://x/webservice/rest/server.php?wstoken=` + secret + `&wsfunction=f": timeout`
	if got := Redact(msg); strings.Contains(got, secret) {
		t.Fatalf("segredo vazou: %s", got)
	}
}

func TestLoggerRedacts(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "json", slog.LevelDebug)
	log.Info("fetch",
		"url", "https://x/?authtoken="+secret,
		"moodle_token", secret,
		"err", errors.New("falhou em https://x/?wstoken="+secret),
	)
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("segredo vazou no log: %s", buf.String())
	}
}
