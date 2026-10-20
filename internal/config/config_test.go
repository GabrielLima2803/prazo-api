package config

import (
	"testing"

	"prazo/internal/domain"
)

func TestLoadFilters(t *testing.T) {
	t.Setenv("PRAZO_IGNORE_COURSES", "est, 2026/2_CCT_CCI251-03U_CDI2002")
	t.Setenv("PRAZO_IGNORE_TYPES", "forum,Other")
	t.Setenv("PRAZO_IGNORE_TITLE_REGEX", "(?i)opcional")
	t.Setenv("PRAZO_INCLUDE_OPENINGS", "true")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Filter.Courses["EST"]; !ok {
		t.Error("EST deveria estar nas disciplinas ignoradas")
	}
	if _, ok := c.Filter.Types[domain.TypeOther]; !ok {
		t.Error("other deveria estar nos tipos ignorados")
	}
	if !c.Filter.Titles.MatchString("Leitura OPCIONAL") || !c.IncludeOpenings {
		t.Errorf("config = %+v", c)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"PRAZO_IGNORE_TYPES":       "prova",
		"PRAZO_IGNORE_TITLE_REGEX": "(",
		"PRAZO_ICAL_URL":           "http://sem-https?authtoken=x",
		"PRAZO_LOG_FORMAT":         "xml",
		"PRAZO_INCLUDE_OPENINGS":   "talvez",
	}
	for k, v := range cases {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%q deveria ser rejeitado", k, v)
			}
		})
	}
}
