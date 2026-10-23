package config

import (
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"prazo/internal/domain"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Moodle.BaseURL != DefaultMoodleURL || cfg.Log.Format != "text" || cfg.Location.String() != DefaultTimezone {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Google.CalendarName != DefaultCalendarName || cfg.Storage.DataDir != DefaultDataDir || cfg.Storage.MasterKey != nil {
		t.Errorf("google/storage = %+v %+v", cfg.Google, cfg.Storage)
	}
	if !cfg.Normalize.Filter.Since.IsZero() {
		t.Error("sem PRAZO_SYNC_SINCE não deveria haver corte")
	}
}

func TestLoadGoogleAndStorage(t *testing.T) {
	cfg, err := load(envFrom(map[string]string{
		"PRAZO_MASTER_KEY":         "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"PRAZO_GOOGLE_CREDENTIALS": "/run/secrets/google.json",
		"PRAZO_DATA_DIR":           "/data",
		"PRAZO_SYNC_SINCE":         "2026-08-01",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Storage.MasterKey) != 32 || cfg.Storage.DataDir != "/data" || cfg.Google.CredentialsFile != "/run/secrets/google.json" {
		t.Errorf("cfg = %+v %+v", cfg.Google, cfg.Storage)
	}
	if want := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC); !cfg.Normalize.Filter.Since.Equal(want) {
		t.Errorf("Since = %v; quer %v (meia-noite em São Paulo)", cfg.Normalize.Filter.Since, want)
	}
}

func TestLoadDerivesBaseURLFromICal(t *testing.T) {
	cfg, err := load(envFrom(map[string]string{
		"PRAZO_ICAL_URL": "https://www.moodle.udesc.br/calendar/export_execute.php?authtoken=x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Moodle.BaseURL != "https://www.moodle.udesc.br" {
		t.Errorf("BaseURL = %q", cfg.Moodle.BaseURL)
	}
}

func TestLoadFilters(t *testing.T) {
	cfg, err := load(envFrom(map[string]string{
		"PRAZO_IGNORE_COURSES":     "est, 2026/2_CCT_CCI251-03U_CDI2002",
		"PRAZO_IGNORE_TYPES":       "forum,Other",
		"PRAZO_IGNORE_TITLE_REGEX": "(?i)opcional",
		"PRAZO_INCLUDE_OPENINGS":   "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	f := cfg.Normalize.Filter
	if !f.Courses.Has("EST") || !f.Types.Has(domain.TypeOther) || !f.Titles.MatchString("Leitura OPCIONAL") {
		t.Errorf("filter = %+v", f)
	}
	if !cfg.Normalize.IncludeOpenings {
		t.Error("IncludeOpenings deveria ser true")
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := load(envFrom(map[string]string{
		"PRAZO_IGNORE_TYPES":       "prova",
		"PRAZO_IGNORE_TITLE_REGEX": "(",
		"PRAZO_ICAL_URL":           "http://sem-https/x?authtoken=SEGREDO",
		"PRAZO_LOG_FORMAT":         "xml",
		"PRAZO_INCLUDE_OPENINGS":   "talvez",
		"PRAZO_TIMEZONE":           "Marte/Olympus",
		"PRAZO_SYNC_SINCE":         "01/08/2026",
		"PRAZO_MASTER_KEY":         "c2VncmVkby1jdXJ0bw==",
	}))
	if err == nil {
		t.Fatal("esperava erro")
	}
	msg := err.Error()
	for _, key := range []string{"PRAZO_IGNORE_TYPES", "PRAZO_IGNORE_TITLE_REGEX", "PRAZO_ICAL_URL", "PRAZO_LOG_FORMAT", "PRAZO_INCLUDE_OPENINGS", "PRAZO_TIMEZONE", "PRAZO_SYNC_SINCE", "PRAZO_MASTER_KEY"} {
		if !strings.Contains(msg, key) {
			t.Errorf("erro não menciona %s:\n%s", key, msg)
		}
	}
	if strings.Contains(msg, "SEGREDO") || strings.Contains(msg, "c2VncmVkby1jdXJ0bw") {
		t.Error("o erro não pode ecoar a URL do iCal")
	}
}
