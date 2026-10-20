// Package config lê a configuração de variáveis de ambiente (ver .env.example).
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
)

const DefaultMoodleURL = "https://www.moodle.udesc.br"

type Config struct {
	ICalURL     string // segredo: contém authtoken
	MoodleURL   string
	MoodleToken string // segredo
	LogFormat   string // "json" ou "text"
	LogLevel    slog.Level
}

func Load() (Config, error) {
	c := Config{
		ICalURL:     os.Getenv("PRAZO_ICAL_URL"),
		MoodleURL:   strings.TrimRight(getenv("PRAZO_MOODLE_URL", DefaultMoodleURL), "/"),
		MoodleToken: os.Getenv("PRAZO_MOODLE_TOKEN"),
		LogFormat:   getenv("PRAZO_LOG_FORMAT", "text"),
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return Config{}, fmt.Errorf("PRAZO_LOG_FORMAT deve ser json ou text, veio %q", c.LogFormat)
	}
	if err := c.LogLevel.UnmarshalText([]byte(getenv("PRAZO_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("PRAZO_LOG_LEVEL: %w", err)
	}
	if c.ICalURL != "" {
		u, err := url.Parse(c.ICalURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			// Não ecoar o valor: ele contém o authtoken.
			return Config{}, fmt.Errorf("PRAZO_ICAL_URL inválida (precisa ser https://...)")
		}
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
