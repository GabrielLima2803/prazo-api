package domain

import (
	"strings"
	"unicode"
)

// CourseCode extrai a sigla da disciplina do shortname do Moodle.
//
// Formato observado no Moodle do CCT (2026-10):
//
//	2026/2_CCT_CCI251-03A_POO0002 → POO
//
// ou seja, semestre_centro_turma_código, e o código é letras + dígitos.
// Para formatos desconhecidos, usa o primeiro bloco de letras do último
// segmento; se não houver letras, devolve o shortname inteiro.
func CourseCode(shortname string) string {
	s := strings.TrimSpace(shortname)
	if s == "" {
		return ""
	}
	last := s
	if i := strings.LastIndexAny(s, "_ "); i >= 0 && i < len(s)-1 {
		last = s[i+1:]
	}
	letters := strings.TrimRightFunc(last, func(r rune) bool { return !unicode.IsLetter(r) })
	if j := strings.IndexFunc(letters, func(r rune) bool { return !unicode.IsLetter(r) }); j >= 0 {
		letters = letters[:j]
	}
	if letters == "" {
		return s
	}
	return strings.ToUpper(letters)
}
