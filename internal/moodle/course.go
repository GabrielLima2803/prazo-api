package moodle

import (
	"strings"
	"unicode"
)

// CourseCode extrai a sigla do shortname da disciplina.
//
// Formato do CCT: "2026/2_CCT_CCI251-03A_POO0002" → "POO" (semestre_centro_turma_código).
// Para outros formatos, usa o primeiro bloco de letras do último segmento e,
// sem letras, devolve o shortname inteiro.
func CourseCode(shortname string) string {
	s := strings.TrimSpace(shortname)
	if s == "" {
		return ""
	}
	last := s
	if i := strings.LastIndexAny(s, "_ "); i >= 0 && i < len(s)-1 {
		last = s[i+1:]
	}
	start := strings.IndexFunc(last, unicode.IsLetter)
	if start < 0 {
		return s
	}
	letters := last[start:]
	if end := strings.IndexFunc(letters, func(r rune) bool { return !unicode.IsLetter(r) }); end >= 0 {
		letters = letters[:end]
	}
	return strings.ToUpper(letters)
}
