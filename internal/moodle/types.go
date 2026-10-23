// Package moodle interpreta convenções do Moodle (nomes de evento, shortnames
// de disciplina). É puro: não faz I/O e é compartilhado pelas fontes.
package moodle

import "github.com/GabrielLima2803/prazo-sync/internal/domain"

// EventName é o resultado de interpretar o nome de um evento do calendário.
type EventName struct {
	Type  domain.DeadlineType
	Phase domain.Phase
	Title string // nome da atividade, sem o texto do template
	Rule  string // regra que casou; vazio se nenhuma
}

// nameTemplate é um texto que o Moodle adiciona ao nome da atividade ao criar
// o evento, já traduzido para o idioma do site.
type nameTemplate struct {
	rule   string
	prefix string
	suffix string
	typ    domain.DeadlineType
	phase  domain.Phase
}
