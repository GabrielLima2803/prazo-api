// Package config carrega a configuração das variáveis de ambiente
// (documentadas em .env.example). Todos os erros são reportados de uma vez.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"prazo/internal/collections"
	"prazo/internal/crypto"
	"prazo/internal/domain"
)

const (
	DefaultMoodleURL    = "https://moodle.joinville.udesc.br"
	DefaultTimezone     = "America/Sao_Paulo"
	DefaultCalendarName = "Prazo — UDESC"
	DefaultDataDir      = "data"
)

type Config struct {
	Moodle    Moodle
	Google    Google
	Storage   Storage
	Log       Log
	Normalize domain.NormalizeOptions
	Location  *time.Location // fuso de exibição e dos eventos na agenda
}

type Google struct {
	CredentialsFile string // JSON da credencial OAuth "App para computador"
	CalendarName    string
}

type Storage struct {
	DataDir   string
	MasterKey []byte // segredo; nil quando PRAZO_MASTER_KEY não foi definida
}

type Moodle struct {
	BaseURL string
	ICalURL string // segredo: contém o authtoken
	Token   string // segredo
}

type Log struct {
	Format string // json | text
	Level  slog.Level
}

// Load lê o ambiente do processo.
func Load() (Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	cfg := Config{
		Moodle: Moodle{
			ICalURL: e.str("PRAZO_ICAL_URL", ""),
			Token:   e.str("PRAZO_MOODLE_TOKEN", ""),
		},
		Google: Google{
			CredentialsFile: e.str("PRAZO_GOOGLE_CREDENTIALS", ""),
			CalendarName:    e.str("PRAZO_CALENDAR_NAME", DefaultCalendarName),
		},
		Storage: Storage{DataDir: e.str("PRAZO_DATA_DIR", DefaultDataDir)},
		Log:     Log{Format: e.oneOf("PRAZO_LOG_FORMAT", "text", "text", "json")},
	}
	if raw := e.str("PRAZO_MASTER_KEY", ""); raw != "" {
		key, err := crypto.ParseKey(raw)
		if err != nil {
			e.fail("PRAZO_MASTER_KEY", err) // sem ecoar o valor
		}
		cfg.Storage.MasterKey = key
	}
	cfg.Moodle.BaseURL = strings.TrimRight(e.str("PRAZO_MOODLE_URL", baseURLOf(cfg.Moodle.ICalURL)), "/")
	if cfg.Moodle.ICalURL != "" && baseURLOf(cfg.Moodle.ICalURL) == "" {
		e.fail("PRAZO_ICAL_URL", errors.New("precisa ser uma URL https://")) // sem ecoar: contém o authtoken
	}
	if err := cfg.Log.Level.UnmarshalText([]byte(e.str("PRAZO_LOG_LEVEL", "info"))); err != nil {
		e.fail("PRAZO_LOG_LEVEL", err)
	}

	loc, err := time.LoadLocation(e.str("PRAZO_TIMEZONE", DefaultTimezone))
	if err != nil {
		e.fail("PRAZO_TIMEZONE", err)
		loc = time.UTC
	}
	cfg.Location = loc

	cfg.Normalize = domain.NormalizeOptions{
		Filter: domain.Filter{
			Courses: e.upperSet("PRAZO_IGNORE_COURSES"),
			Types:   e.typeSet("PRAZO_IGNORE_TYPES"),
			Titles:  e.regexp("PRAZO_IGNORE_TITLE_REGEX"),
			Since:   e.date("PRAZO_SYNC_SINCE", loc),
		},
		IncludeOpenings: e.bool("PRAZO_INCLUDE_OPENINGS", false),
	}

	if err := errors.Join(e.errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// baseURLOf devolve "https://host" de uma URL https, ou DefaultMoodleURL se raw for vazia.
func baseURLOf(raw string) string {
	if raw == "" {
		return DefaultMoodleURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

type env struct {
	get  func(string) string
	errs []error
}

func (e *env) fail(key string, err error) { e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err)) }

func (e *env) str(key, def string) string {
	if v := strings.TrimSpace(e.get(key)); v != "" {
		return v
	}
	return def
}

func (e *env) oneOf(key, def string, allowed ...string) string {
	v := e.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	e.fail(key, fmt.Errorf("valor %q inválido (use %s)", v, strings.Join(allowed, " ou ")))
	return def
}

func (e *env) bool(key string, def bool) bool {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.fail(key, err)
		return def
	}
	return b
}

func (e *env) list(key string) []string {
	var out []string
	for p := range strings.SplitSeq(e.get(key), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (e *env) upperSet(key string) collections.Set[string] {
	s := collections.Set[string]{}
	for _, v := range e.list(key) {
		s.Add(strings.ToUpper(v))
	}
	return s
}

func (e *env) typeSet(key string) collections.Set[domain.DeadlineType] {
	s := collections.Set[domain.DeadlineType]{}
	for _, v := range e.list(key) {
		t, err := domain.ParseDeadlineType(v)
		if err != nil {
			e.fail(key, err)
			continue
		}
		s.Add(t)
	}
	return s
}

// date lê AAAA-MM-DD como meia-noite no fuso loc.
func (e *env) date(key string, loc *time.Location) time.Time {
	v := e.str(key, "")
	if v == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation(time.DateOnly, v, loc)
	if err != nil {
		e.fail(key, fmt.Errorf("use o formato AAAA-MM-DD: %w", err))
	}
	return t.UTC()
}

func (e *env) regexp(key string) *regexp.Regexp {
	expr := e.str(key, "")
	if expr == "" {
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		e.fail(key, err)
	}
	return re
}
