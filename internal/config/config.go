// Package config lê a configuração de variáveis de ambiente (ver .env.example).
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"prazo/internal/domain"
)

const DefaultMoodleURL = "https://moodle.joinville.udesc.br"

type Config struct {
	ICalURL     string // segredo: contém authtoken
	MoodleURL   string
	MoodleToken string // segredo
	LogFormat   string // "json" ou "text"
	LogLevel    slog.Level

	Filter          domain.Filter
	IncludeOpenings bool
}

var validTypes = map[domain.DeadlineType]struct{}{
	domain.TypeAssign: {}, domain.TypeQuiz: {}, domain.TypeForum: {},
	domain.TypeExam: {}, domain.TypeCourseEvent: {}, domain.TypeOther: {},
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
	var err error
	if err = c.LogLevel.UnmarshalText([]byte(getenv("PRAZO_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("PRAZO_LOG_LEVEL: %w", err)
	}
	if c.ICalURL != "" {
		u, err := url.Parse(c.ICalURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			// Não ecoar o valor: ele contém o authtoken.
			return Config{}, fmt.Errorf("PRAZO_ICAL_URL inválida (precisa ser https://...)")
		}
	}
	if c.Filter, err = loadFilter(); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("PRAZO_INCLUDE_OPENINGS"); v != "" {
		if c.IncludeOpenings, err = strconv.ParseBool(v); err != nil {
			return Config{}, fmt.Errorf("PRAZO_INCLUDE_OPENINGS: %w", err)
		}
	}
	return c, nil
}

func loadFilter() (domain.Filter, error) {
	f := domain.Filter{Courses: map[string]struct{}{}, Types: map[domain.DeadlineType]struct{}{}}
	for _, c := range splitList(os.Getenv("PRAZO_IGNORE_COURSES")) {
		f.Courses[strings.ToUpper(c)] = struct{}{}
	}
	for _, t := range splitList(os.Getenv("PRAZO_IGNORE_TYPES")) {
		typ := domain.DeadlineType(strings.ToLower(t))
		if _, ok := validTypes[typ]; !ok {
			return f, fmt.Errorf("PRAZO_IGNORE_TYPES: tipo desconhecido %q", t)
		}
		f.Types[typ] = struct{}{}
	}
	if expr := os.Getenv("PRAZO_IGNORE_TITLE_REGEX"); expr != "" {
		re, err := regexp.Compile(expr)
		if err != nil {
			return f, fmt.Errorf("PRAZO_IGNORE_TITLE_REGEX: %w", err)
		}
		f.Titles = re
	}
	return f, nil
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
